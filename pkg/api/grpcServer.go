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

package api

// gRPC server. The GRPCServer type handles incoming streaming gRPC traffic and invokes method-specific
// handlers to implement the method-specific logic.
//
// When you create a GRPCServer instance, you specify a number of transport-level options, along with the
// set of method handlers responsible for the logic of individual API methods

// TODO: Once the gRPC code is updated to use context objects from "context" as
// opposed to from "golang.org/x/net/context", this code should be updated to
// pass the context from the gRPC streams to downstream calls as opposed to merely
// using context.Background.

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net"

	"github.com/golang/glog"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"istio.io/mixer/pkg/attribute"

	proto "github.com/golang/protobuf/proto"
	mixerpb "istio.io/api/mixer/api/v1"
)

// GRPCServerOptions controls the behavior of a gRPC server.
type GRPCServerOptions struct {
	// MaximumMessageSize constrains the size of incoming requests.
	MaxMessageSize uint

	// MaxConcurrentStreams limits the amount of concurrency allowed,
	// in order to put a cap on server-side resource usage.
	MaxConcurrentStreams uint

	// Port specifies the IP port the server should listen on.
	Port uint16

	// CompressedPayload determines whether compression should be
	// used on individual messages.
	CompressedPayload bool

	// ServerCertificate provides the server-side cert for TLS connections.
	// If this is not supplied, only connections in the clear are supported.
	ServerCertificate *tls.Certificate

	// ClientCertificate provides the acceptable client-side certs. Only clients
	// presenting one of these certs will be allowed to connect. If this is nil,
	// then any clients will be allowed.
	ClientCertificates *x509.CertPool

	// Handlers holds pointers to the functions that implement request-level processing
	// for all API methods
	Handlers MethodHandlers

	// AttributeManager holds a pointer to an initialized AttributeManager to use when
	// processing incoming attribute requests.
	AttributeManager attribute.Manager
}

// GRPCServer holds the state for the gRPC API server.
// Use NewGRPCServer to get one of these.
type GRPCServer struct {
	server   *grpc.Server
	listener net.Listener
	handlers MethodHandlers
	attrMgr  attribute.Manager
}

// NewGRPCServer creates the gRPC serving stack.
func NewGRPCServer(options *GRPCServerOptions) (*GRPCServer, error) {
	// get the network stuff setup
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", options.Port))
	if err != nil {
		return nil, err
	}

	// construct the gRPC options

	var grpcOptions []grpc.ServerOption
	grpcOptions = append(grpcOptions, grpc.MaxConcurrentStreams(uint32(options.MaxConcurrentStreams)))
	grpcOptions = append(grpcOptions, grpc.MaxMsgSize(int(options.MaxMessageSize)))

	if options.CompressedPayload {
		grpcOptions = append(grpcOptions, grpc.RPCCompressor(grpc.NewGZIPCompressor()))
		grpcOptions = append(grpcOptions, grpc.RPCDecompressor(grpc.NewGZIPDecompressor()))
	}

	if options.ServerCertificate != nil {
		// enable TLS
		tlsConfig := &tls.Config{}
		tlsConfig.Certificates = []tls.Certificate{*options.ServerCertificate}

		if options.ClientCertificates != nil {
			// enable TLS mutual auth
			tlsConfig.ClientCAs = options.ClientCertificates
			tlsConfig.ClientAuth = tls.RequireAndVerifyClientCert
		}
		tlsConfig.BuildNameToCertificate()

		grpcOptions = append(grpcOptions, grpc.Creds(credentials.NewTLS(tlsConfig)))
	}

	// get everything wired up
	grpcServer := grpc.NewServer(grpcOptions...)
	s := &GRPCServer{grpcServer, listener, options.Handlers, options.AttributeManager}
	mixerpb.RegisterMixerServer(grpcServer, s)
	return s, nil
}

// Start listening for incoming requests. Only returns
// in catastrophic failure cases.
func (s *GRPCServer) Start() error {
	return s.server.Serve(s.listener)
}

// Stop undoes the effect of a previous call to Listen, basically it stops the server
// from processing any more requests
func (s *GRPCServer) Stop() {
	s.server.GracefulStop()
}

type handlerFunc func(tracker attribute.Tracker, request proto.Message, response proto.Message)

func (s *GRPCServer) streamLoop(stream grpc.ServerStream, request proto.Message, response proto.Message, handler handlerFunc) error {
	tracker := s.attrMgr.NewTracker()
	defer tracker.Done()

	for {
		// get a single message
		if err := stream.RecvMsg(request); err == io.EOF {
			return nil
		} else if err != nil {
			glog.Errorf("Stream error %s", err)
			return err
		}

		// do the actual work for the message
		handler(tracker, request, response)

		// produce the response
		if err := stream.SendMsg(response); err != nil {
			return err
		}

		// reset everything to 0
		request.Reset()
		response.Reset()
	}
}

// Check is the entry point for the external Check method
func (s *GRPCServer) Check(stream mixerpb.Mixer_CheckServer) error {
	return s.streamLoop(stream,
		new(mixerpb.CheckRequest),
		new(mixerpb.CheckResponse),
		func(tracker attribute.Tracker, request proto.Message, response proto.Message) {
			s.handlers.Check(context.Background(), tracker, request.(*mixerpb.CheckRequest), response.(*mixerpb.CheckResponse))
		})
}

// Report is the entry point for the external Report method
func (s *GRPCServer) Report(stream mixerpb.Mixer_ReportServer) error {
	return s.streamLoop(stream,
		new(mixerpb.ReportRequest),
		new(mixerpb.ReportResponse),
		func(tracker attribute.Tracker, request proto.Message, response proto.Message) {
			s.handlers.Report(context.Background(), tracker, request.(*mixerpb.ReportRequest), response.(*mixerpb.ReportResponse))
		})
}

// Quota is the entry point for the external Quota method
func (s *GRPCServer) Quota(stream mixerpb.Mixer_QuotaServer) error {
	return s.streamLoop(stream,
		new(mixerpb.QuotaRequest),
		new(mixerpb.QuotaResponse),
		func(tracker attribute.Tracker, request proto.Message, response proto.Message) {
			s.handlers.Quota(context.Background(), tracker, request.(*mixerpb.QuotaRequest), response.(*mixerpb.QuotaResponse))
		})
}
