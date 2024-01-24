package config

import (
	"context"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"zombiezen.com/go/sqlite/sqlitex"
)

type Arguments struct {
	Verbose           bool
	Pool              *sqlitex.Pool
	StoreChannel      chan *unstructured.Unstructured
	FatalErrorChannel chan error
	CancelFunc        context.CancelCauseFunc
}
