package config

import (
	"context"
	"database/sql"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type Arguments struct {
	Verbose           bool
	Db                *sql.DB
	StoreChannel      chan *unstructured.Unstructured
	FatalErrorChannel chan error
	CancelFunc        context.CancelCauseFunc
}
