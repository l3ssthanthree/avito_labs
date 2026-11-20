//go:generate go tool mockgen -source ${GOFILE} -destination mocks_test.go -package ${GOPACKAGE}_test

package ready

import "context"

type databasePinger interface {
	Ping(ctx context.Context) error
}
