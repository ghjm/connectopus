//go:build tools
// +build tools

package tools

//nolint:typecheck
import _ "github.com/99designs/gqlgen" // Generator for GraphQL server
import _ "github.com/Yamashou/gqlgenc" // Generator for GraphQL client
