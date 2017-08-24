package server

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/containous/mux"
	"github.com/containous/traefik/testhelpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseOneRule(t *testing.T) {
	router := mux.NewRouter()
	routes := make([]*mux.Route, 1)
	routes[0] = router.NewRoute()
	serverRoute := &serverRoute{routes: routes}
	rules := &Rules{route: serverRoute}

	expression := "Host:foo.bar"
	routeResults, err := rules.Parse(expression)
	require.NoError(t, err, "Error while building route for %s", expression)

	request := testhelpers.MustNewRequest(http.MethodGet, "http://foo.bar", nil)

	routeMatch := false
	for _, routeResult := range routeResults {
		if !routeMatch {
			routeMatch := routeResult.Match(request, &mux.RouteMatch{Route: routeResult})
		}
	}

	assert.True(t, routeMatch, "Rule %s don't match.", expression)
}

func TestParseTwoRules(t *testing.T) {
	router := mux.NewRouter()
	routes := make([]*mux.Route, 1)
	routes[0] = router.NewRoute()
	serverRoute := &serverRoute{routes: routes}
	rules := &Rules{route: serverRoute}

	expression := "Host: Foo.Bar ; Path:/FOObar"
	routeResults, err := rules.Parse(expression)

	require.NoError(t, err, "Error while building route for %s.", expression)

	routeMatch := false
	request := testhelpers.MustNewRequest(http.MethodGet, "http://foo.bar/foobar", nil)
	for _, routeResult := range routeResults {
		if !routeMatch {
			routeMatch := routeResult.Match(request, &mux.RouteMatch{Route: routeResult})
		}
	}

	assert.False(t, routeMatch, "Rule %s don't match.", expression)

	routeMatch = false
	request = testhelpers.MustNewRequest(http.MethodGet, "http://foo.bar/FOObar", nil)
	for _, routeResult := range routeResults {
		if !routeMatch {
			routeMatch = routeResult.Match(request, &mux.RouteMatch{Route: routeResult})
		}
	}

	assert.True(t, routeMatch, "Rule %s don't match.", expression)
}

func TestParseDomains(t *testing.T) {
	rules := &Rules{}

	tests := []struct {
		expression string
		domain     []string
	}{
		{
			expression: "Host:foo.bar,test.bar",
			domain:     []string{"foo.bar", "test.bar"},
		},
		{
			expression: "Path:/test",
			domain:     []string{},
		},
		{
			expression: "Host:foo.bar;Path:/test",
			domain:     []string{"foo.bar"},
		},
		{
			expression: "Host: Foo.Bar ;Path:/test",
			domain:     []string{"foo.bar"},
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.expression, func(t *testing.T) {
			t.Parallel()

			domains, err := rules.ParseDomains(test.expression)
			require.NoError(t, err, "%s: Error while parsing domain.", test.expression)

			assert.EqualValues(t, test.domain, domains, "%s: Error parsing domains from expression.", test.expression)
		})
	}
}

func TestPriorites(t *testing.T) {
	router := mux.NewRouter()
	routes := make([]*mux.Route, 1)
	routes[0] = router.NewRoute()
	router.StrictSlash(true)
	rules := &Rules{route: &serverRoute{routes: routes}}
	expression01 := "PathPrefix:/foo"

	routesFoo, err := rules.Parse(expression01)
	require.NoError(t, err, "Error while building route for %s", expression01)

	fooHandler := &fakeHandler{name: "fooHandler"}
	for idx := range routesFoo {
		routesFoo[idx].Handler(fooHandler)
	}

	routeMatch := router.Match(&http.Request{URL: &url.URL{Path: "/foo"}}, &mux.RouteMatch{})
	assert.True(t, routeMatch, "Error matching route")

	routeMatch = router.Match(&http.Request{URL: &url.URL{Path: "/fo"}}, &mux.RouteMatch{})
	assert.False(t, routeMatch, "Error matching route")

	multipleRules := &Rules{route: &serverRoute{routes: routes}}
	expression02 := "PathPrefix:/foobar"

	routesFoobar, err := multipleRules.Parse(expression02)
	require.NoError(t, err, "Error while building route for %s", expression02)

	foobarHandler := &fakeHandler{name: "foobarHandler"}
	for idx := range routesFooBar {
		routesFooBar[idx].Handler(foobarHandler)
	}
	routesFoobar.Handler(foobarHandler)
	routeMatch = router.Match(&http.Request{URL: &url.URL{Path: "/foo"}}, &mux.RouteMatch{})

	assert.True(t, routeMatch, "Error matching route")

	fooMatcher := &mux.RouteMatch{}
	routeMatch = router.Match(&http.Request{URL: &url.URL{Path: "/foobar"}}, fooMatcher)

	assert.True(t, routeMatch, "Error matching route")
	assert.NotEqual(t, fooMatcher.Handler, foobarHandler, "Error matching priority")
	assert.Equal(t, fooMatcher.Handler, fooHandler, "Error matching priority")

	for idx := range routesFoo {
		routesFoo[idx].Priority(1)
	}
	for idx := range routesFooBar {
		routesFoobar[idx].Priority(10)
	}
	router.SortRoutes()

	foobarMatcher := &mux.RouteMatch{}
	routeMatch = router.Match(&http.Request{URL: &url.URL{Path: "/foobar"}}, foobarMatcher)

	assert.True(t, routeMatch, "Error matching route")
	assert.Equal(t, foobarMatcher.Handler, foobarHandler, "Error matching priority")
	assert.NotEqual(t, foobarMatcher.Handler, fooHandler, "Error matching priority")
}

type fakeHandler struct {
	name string
}

func (h *fakeHandler) ServeHTTP(http.ResponseWriter, *http.Request) {}
