//go:generate go tool mockgen -source ${GOFILE} -destination mocks_test.go -package ${GOPACKAGE}

package push

import "net/http"

type httpClient interface {
	Do(request *http.Request) (*http.Response, error)
}
