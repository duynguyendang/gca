package promptbuilder

import (
	"github.com/duynguyendang/meb"
)

type PromptStore interface {
	GetStore(projectID string) (*meb.MEBStore, error)
	GetAnalyticalStore(projectID string) (*meb.MEBStore, error)
}