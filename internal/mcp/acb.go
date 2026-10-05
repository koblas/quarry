package mcp

import "context"

// acbStub is the acb tool's document until its handler reads the store.
type acbStub struct {
	AsOf     string   `json:"as_of"`
	Warnings []string `json:"warnings"`
}

// acb is the acb tool.
func (s *Server) acb(_ context.Context, _ acbInput) (any, error) {
	return acbStub{Warnings: []string{}}, nil
}
