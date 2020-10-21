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
	"encoding/json"
	"log"
	"net/http"

	// Also use the default pprof handlers
	_ "net/http/pprof"
)

// ListenAndServe configures the default HTTP mux with pprof and
// additional endpoints for retrieving JSON objects.
func ListenAndServe(bind string, elts map[string]interface{}) error {
	for k, v := range elts {
		// Capture vars.
		k, v := k, v
		http.HandleFunc(k, func(writer http.ResponseWriter, request *http.Request) {
			writer.Header().Add("content-type", "application/json")
			writer.WriteHeader(http.StatusOK)
			enc := json.NewEncoder(writer)
			enc.SetIndent("", "  ")

			if err := enc.Encode(v); err != nil {
				log.Printf("could not serve %s: %v", k, err)
			}
		})

	}
	return http.ListenAndServe(bind, nil)
}
