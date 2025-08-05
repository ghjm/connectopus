package main

import (
	"fmt"
	"github.com/99designs/gqlgen/api"
	"github.com/99designs/gqlgen/codegen/config"
	"github.com/Yamashou/gqlgenc/clientgenv2"
	"os"
)

func main() {
	fmt.Printf("Generating localui...\n")
	packageName := "localui"
	clientOutputFilePath := "./gql_client_gen.go"
	queryFilePaths := []string{"./graphql/*.graphql"}

	cfg, err := config.LoadConfigFromDefaultLocations()
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "failed to load config: %+v\n", err)
		os.Exit(2)
	}

	clientConfig := config.PackageConfig{Filename: clientOutputFilePath, Package: packageName}
	if err = clientConfig.Check(); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "failed to load generated file path: %+v\n", err)
		os.Exit(2)
	}

	clientPlugin := clientgenv2.NewWithQueryDocument(queryFilePaths, clientConfig, nil)
	err = api.Generate(cfg, api.AddPlugin(clientPlugin))
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "%+v\n", err)
		os.Exit(4)
	}
}
