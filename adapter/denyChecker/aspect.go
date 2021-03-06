// Copyright 2016 Google Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package denyChecker

import (
	"google.golang.org/genproto/googleapis/rpc/status"

	"istio.io/mixer/pkg/aspect/denyChecker"

	pb "istio.io/mixer/adapter/denyChecker/config_proto"
)

type aspectState struct {
	status status.Status
}

// newAspect returns a new aspect.
func newAspect(c *pb.Config) (denyChecker.Aspect, error) {
	return &aspectState{
		status: *c.Error,
	}, nil
}

func (a *aspectState) Close() error        { return nil }
func (a *aspectState) Deny() status.Status { return a.status }
