// Copyright 2020 The Cockroach Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//   http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package diagnostics provides diagnostic informationg using the
// default HTTP mux.
package diagnostics

import (
	"fmt"
	"log"
	"net/http"

	// Also use the default pprof handlers
	_ "net/http/pprof"

	"gopkg.in/yaml.v3"
)

// ListenAndServe configures the default HTTP mux with pprof and
// additional endpoints for retrieving YAML objects.
func ListenAndServe(bind string, elts map[string]interface{}) error {
	for k, v := range elts {
		// Capture vars.
		k, v := k, v
		http.HandleFunc(k, func(writer http.ResponseWriter, request *http.Request) {
			writer.Header().Add("content-type", "text/plain; charset=UTF-8")
			writer.WriteHeader(http.StatusOK)
			enc := yaml.NewEncoder(writer)

			if err := enc.Encode(v); err != nil {
				log.Printf("could not serve %s: %v", k, err)
			} else {
				_ = enc.Close()
			}
		})

	}

	http.HandleFunc("/", func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Add("content-type", "text/html; charset=UTF-8")
		writer.WriteHeader(http.StatusOK)

		fmt.Fprint(writer, "<html><body>")
		for k := range elts {
			fmt.Fprintf(writer, `<a href="%s">%s</a><br/>`, k, k)
		}
		fmt.Fprint(writer, `<a href="/debug/pprof">/debug/pprof</a></br/>`)
	})

	return http.ListenAndServe(bind, nil)
}
