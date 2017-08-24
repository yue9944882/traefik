package server

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"reflect"
	"sort"
	"strings"

	"github.com/BurntSushi/ty/fun"
	"github.com/containous/mux"
	"github.com/containous/traefik/types"
)

// Rules holds rule parsing and configuration
type Rules struct {
	route *serverRoute
	err   error
}

func (r *Rules) host(hosts ...string) []*mux.Route {
	for idx := range r.route.routes {
		r.route.routes[idx].MatcherFunc(func(req *http.Request, route *mux.RouteMatch) bool {
			reqHost, _, err := net.SplitHostPort(req.Host)
			if err != nil {
				reqHost = req.Host
			}
			for _, host := range hosts {
				if types.CanonicalDomain(reqHost) == types.CanonicalDomain(host) {
					return true
				}
			}
			return false
		})
	}
	return r.route.routes
}

func (r *Rules) hostRegexp(hosts ...string) []*mux.Route {
	for idx := range r.route.routes {
		router := r.route.routes[idx].Subrouter()
		for _, host := range hosts {
			router.Host(types.CanonicalDomain(host))
		}
	}
	return r.route.routes
}

func (r *Rules) path(paths ...string) []*mux.Route {
	for idx := range r.route.routes {
		router := r.route.routes[idx].Subrouter()
		for _, path := range paths {
			router.Path(strings.TrimSpace(path))
		}
	}
	return r.route.routes
}

func (r *Rules) pathPrefix(paths ...string) []*mux.Route {
	for idx := range r.route.routes {
		router := r.route.routes[idx].Subrouter()
		for _, path := range paths {
			router.PathPrefix(strings.TrimSpace(path))
		}
	}
	return r.route.routes
}

type bySize []string

func (a bySize) Len() int           { return len(a) }
func (a bySize) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a bySize) Less(i, j int) bool { return len(a[i]) > len(a[j]) }

func (r *Rules) pathStrip(paths ...string) []*mux.Route {
	sort.Sort(bySize(paths))
	r.route.stripPrefixes = paths
	for idx := range r.route.routes {
		router := r.route.routes[idx].Subrouter()
		for _, path := range paths {
			router.Path(strings.TrimSpace(path))
		}
	}
	return r.route.routes
}

func (r *Rules) pathStripRegex(paths ...string) []*mux.Route {
	sort.Sort(bySize(paths))
	r.route.stripPrefixesRegex = paths
	for idx := range r.route.routes {
		router := r.route.routes[idx].Subrouter()
		for _, path := range paths {
			router.Path(strings.TrimSpace(path))
		}
	}
	return r.route.routes
}

func (r *Rules) replacePath(paths ...string) []*mux.Route {
	for _, path := range paths {
		r.route.replacePath = path
	}
	return r.route.routes
}

func (r *Rules) addPrefix(paths ...string) []*mux.Route {
	for _, path := range paths {
		r.route.addPrefix = path
	}
	return r.route.routes
}

func (r *Rules) pathPrefixStrip(paths ...string) []*mux.Route {
	sort.Sort(bySize(paths))
	r.route.stripPrefixes = paths
	for idx := range r.route.routes {
		router := r.route.routes[idx].Subrouter()
		for _, path := range paths {
			router.PathPrefix(strings.TrimSpace(path))
		}
	}
	return r.route.routes
}

func (r *Rules) pathPrefixStripRegex(paths ...string) []*mux.Route {
	sort.Sort(bySize(paths))
	r.route.stripPrefixesRegex = paths
	router := r.route.route.Subrouter()
	for idx := range r.route.routes {
		router := r.route.routes[idx].Subrouter()
		for _, path := range paths {
			router.PathPrefix(strings.TrimSpace(path))
		}
	}
	return r.route.routes
}

func (r *Rules) methods(methods ...string) []*mux.Route {
	for idx := r.route.routes {
		r.route.routes[idx].Methods(methods...)
	}
	return r.route.routes
}

func (r *Rules) headers(headers ...string) []*mux.Route {
	for idx := r.route.routes {
		r.route.routes[idx].Headers(headers...)
	}
	return r.route.routes
}

func (r *Rules) headersRegexp(headers ...string) []*mux.Route {
	for idx := r.route.routes {
		r.route.routes[idx].HeadersRegexp(headers...)
	}
	return r.route.routes
}

func (r *Rules) parseRules(expression string, onRule func(functionName string, function interface{}, arguments []string) error) error {
	functions := map[string]interface{}{
		"Host":                 r.host,
		"HostRegexp":           r.hostRegexp,
		"Path":                 r.path,
		"PathStrip":            r.pathStrip,
		"PathStripRegex":       r.pathStripRegex,
		"PathPrefix":           r.pathPrefix,
		"PathPrefixStrip":      r.pathPrefixStrip,
		"PathPrefixStripRegex": r.pathPrefixStripRegex,
		"Method":               r.methods,
		"Headers":              r.headers,
		"HeadersRegexp":        r.headersRegexp,
		"AddPrefix":            r.addPrefix,
		"ReplacePath":          r.replacePath,
	}

	if len(expression) == 0 {
		return errors.New("Empty rule")
	}

	f := func(c rune) bool {
		return c == ':'
	}

	// Allow multiple rules separated by ;
	splitRule := func(c rune) bool {
		return c == ';'
	}

	parsedRules := strings.FieldsFunc(expression, splitRule)

	for _, rule := range parsedRules {
		// get function
		parsedFunctions := strings.FieldsFunc(rule, f)
		if len(parsedFunctions) == 0 {
			return fmt.Errorf("error parsing rule: '%s'", rule)
		}
		functionName := strings.TrimSpace(parsedFunctions[0])
		parsedFunction, ok := functions[functionName]
		if !ok {
			return fmt.Errorf("error parsing rule: '%s'. Unknown function: '%s'", rule, parsedFunctions[0])
		}
		parsedFunctions = append(parsedFunctions[:0], parsedFunctions[1:]...)
		fargs := func(c rune) bool {
			return c == ','
		}
		// get function
		parsedArgs := strings.FieldsFunc(strings.Join(parsedFunctions, ":"), fargs)
		if len(parsedArgs) == 0 {
			return fmt.Errorf("error parsing args from rule: '%s'", rule)
		}

		// Split ',' joined args into separated single arg
		for _, parsedArg := range parsedArgs {
			args := make([]string, 1)
			args[0] = strings.TrimSpace(parsedArg)
			err := onRule(functionName, parsedFunction, args)
			if err != nil {
				return fmt.Errorf("Parsing error on rule: %v", err)
			}
		}
	}
	return nil
}

// Parse parses rules expressions
func (r *Rules) Parse(expression string) ([]*mux.Route, error) {
	var resultRoute []*mux.Route
	err := r.parseRules(expression, func(functionName string, function interface{}, arguments []string) error {
		inputs := make([]reflect.Value, len(arguments))
		for i := range arguments {
			inputs[i] = reflect.ValueOf(arguments[i])
		}
		method := reflect.ValueOf(function)
		if method.IsValid() {
			resultRoutes = method.Call(inputs)[0].Interface().([]*mux.Route)
			if r.err != nil {
				return r.err
			}
			for _, resultRoute := range resultRoutes {
				if resultRoute.GetError() != nil {
					return resultRoute.GetError()
				}
			}
		} else {
			return fmt.Errorf("Method not found: '%s'", functionName)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("error parsing rule: %v", err)
	}
	return resultRoute, nil
}

// ParseDomains parses rules expressions and returns domains
func (r *Rules) ParseDomains(expression string) ([]string, error) {
	domains := []string{}
	err := r.parseRules(expression, func(functionName string, function interface{}, arguments []string) error {
		if functionName == "Host" {
			domains = append(domains, arguments...)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("error parsing domains: %v", err)
	}
	return fun.Map(types.CanonicalDomain, domains).([]string), nil
}
