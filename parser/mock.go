package parser

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"

	"github.com/naoina/denco"
	"github.com/subosito/snowboard/api"
)

type MockTransaction struct {
	Path        string
	Pattern     string
	Method      string
	StatusCode  int
	ContentType string
	Body        string
}

type mockRecord struct {
	Pattern      string
	Method       string
	Transactions []*MockTransaction
}

type mockRouter struct {
	routers map[string]*denco.Router
}

func (mr mockRouter) Router(method string) *denco.Router {
	if r, ok := mr.routers[method]; ok {
		return r
	}

	return nil
}

type MockTransactions []*MockTransaction

func (ms MockTransactions) Router() *mockRouter {
	mr := map[string]*mockRecord{}

	for _, m := range ms {
		s := fmt.Sprintf("%s#%s", m.Method, m.Path)

		if r, ok := mr[s]; ok {
			r.Transactions = append(r.Transactions, m)
		} else {
			mr[s] = &mockRecord{
				Pattern:      m.Path,
				Method:       m.Method,
				Transactions: []*MockTransaction{m},
			}
		}
	}

	mc := map[string][]denco.Record{}

	for _, m := range mr {
		d := denco.Record{
			Key:   m.Pattern,
			Value: m,
		}

		if _, ok := mc[m.Method]; ok {
			mc[m.Method] = append(mc[m.Method], d)
		} else {
			mc[m.Method] = []denco.Record{d}
		}
	}

	mx := map[string]*denco.Router{}

	for k, c := range mc {
		r := denco.New()
		r.Build(c)
		mx[k] = r
	}

	return &mockRouter{mx}
}

func Mock(b *api.API) []*MockTransaction {
	ms := []*MockTransaction{}

	for _, g := range b.ResourceGroups {
		for _, x := range g.Resources {
			for _, t := range x.Transitions {
				for _, n := range t.Transactions {
					p := transformURL(t.URL, b.Host())
					m := &MockTransaction{
						Path:        urlPath(p),
						Pattern:     p,
						Method:      n.Request.Method,
						StatusCode:  n.Response.StatusCode,
						ContentType: n.Response.Body.ContentType,
						Body:        n.Response.Body.Body,
					}

					ms = append(ms, m)
				}
			}
		}
	}

	return ms
}

func MockHandler(ms MockTransactions) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		var n *MockTransaction

		z := ms.Router()
		router := z.Router(r.Method)
		data, _, found := router.Lookup(r.URL.Path)
		if !found {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		m := data.(*mockRecord)
		s := r.Header.Get("X-Status-Code")

		if s == "" {
			for _, t := range m.Transactions {
				if t.StatusCode >= http.StatusOK && t.StatusCode < http.StatusBadRequest {
					n = t
				}
			}
		} else {
			for _, t := range m.Transactions {
				if s == strconv.Itoa(t.StatusCode) {
					n = t
				}
			}
		}

		if n == nil {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		log.Printf("%s\t%d\t%s\n", n.Method, n.StatusCode, n.Path)

		w.Header().Set("Content-Type", n.ContentType)
		w.WriteHeader(n.StatusCode)
		io.WriteString(w, n.Body)
	}

	return http.HandlerFunc(fn)
}

func transformURL(u, h string) string {
	paramPattern := regexp.MustCompile(`\{\?[\w,]+\}`)
	queryPattern := regexp.MustCompile(`\{([\w,]+)\}`)

	u = queryPattern.ReplaceAllString(u, ":${1}")
	u = paramPattern.ReplaceAllLiteralString(u, "")
	u = strings.Replace(u, h, "", 1)
	u = path.Join("/", u)

	return u
}

func urlPath(u string) string {
	if x, err := url.Parse(u); err == nil {
		return x.Path
	}

	return u
}
