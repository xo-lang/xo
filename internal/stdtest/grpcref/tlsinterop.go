package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
)

// The mutual TLS oracle (TestGrpcInterop's tls case, map.txt grpc.behavior.mtls): `serve-tls DIR` serves /t.S/Who
// over TLS asking for a client certificate verified by DIR/ca.pem; `client-tls ADDR DIR` calls it with DIR's client
// certificate and per-RPC credentials that require transport security. DIR holds ca.pem, server.pem, server.key,
// client.pem and client.key, written by the test.

func tlsFiles(dir string) (*x509.CertPool, tls.Certificate, tls.Certificate) {
	caPEM, err := os.ReadFile(filepath.Join(dir, "ca.pem"))
	if err != nil {
		panic(err)
	}
	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM(caPEM)
	srv, err := tls.LoadX509KeyPair(filepath.Join(dir, "server.pem"), filepath.Join(dir, "server.key"))
	if err != nil {
		panic(err)
	}
	cli, err := tls.LoadX509KeyPair(filepath.Join(dir, "client.pem"), filepath.Join(dir, "client.key"))
	if err != nil {
		panic(err)
	}
	return pool, srv, cli
}

// whoAnswer is the line /t.S/Who answers: the peer's AuthInfo as the handler sees it, and the call's authorization.
func whoAnswer(ctx context.Context) string {
	p, _ := peer.FromContext(ctx)
	md, _ := metadata.FromIncomingContext(ctx)
	auth := strings.Join(md.Get("authorization"), ",")
	ti, ok := p.AuthInfo.(credentials.TLSInfo)
	if !ok {
		return "auth " + p.AuthInfo.AuthType()
	}
	cn := "none"
	if len(ti.State.PeerCertificates) > 0 {
		cn = ti.State.PeerCertificates[0].Subject.CommonName
	}
	return fmt.Sprintf("auth %s %s %s client=%s chains=%d authorization=%s", ti.AuthType(), ti.SecurityLevel, tls.VersionName(ti.State.Version), cn, len(ti.State.VerifiedChains), auth)
}

func tlsServe(dir string) {
	pool, srvCert, _ := tlsFiles(dir)
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(err)
	}
	fmt.Println(l.Addr().String())
	creds := credentials.NewTLS(&tls.Config{Certificates: []tls.Certificate{srvCert}, ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: pool})
	who := func(_ any, s grpc.ServerStream) error {
		if _, err := recvMsg(s); err != nil {
			return err
		}
		return sendMsg(s, []byte(whoAnswer(s.Context())))
	}
	srv := grpc.NewServer(grpc.Creds(creds), grpc.UnknownServiceHandler(who), grpc.ForceServerCodecV2(bytesCodec{}))
	_ = srv.Serve(l)
}

// tokenCreds are per-RPC credentials that need TLS; each call records what GetRequestMetadata saw.
type tokenCreds struct{ seen *[]string }

func (c tokenCreds) GetRequestMetadata(ctx context.Context, uri ...string) (map[string]string, error) {
	ri, _ := credentials.RequestInfoFromContext(ctx)
	level := "none"
	if ci, ok := ri.AuthInfo.(interface {
		GetCommonAuthInfo() credentials.CommonAuthInfo
	}); ok {
		level = ci.GetCommonAuthInfo().SecurityLevel.String()
	}
	*c.seen = append(*c.seen, fmt.Sprintf("%s %s %s %s", ri.Method, strings.Join(uri, ","), ri.AuthInfo.AuthType(), level))
	return map[string]string{"authorization": "Bearer tls-token"}, nil
}

func (tokenCreds) RequireTransportSecurity() bool { return true }

func tlsClient(addr, dir string) {
	pool, _, cliCert := tlsFiles(dir)
	var seen []string
	creds := credentials.NewTLS(&tls.Config{RootCAs: pool, Certificates: []tls.Certificate{cliCert}})
	cc, err := grpc.NewClient(addr, grpc.WithTransportCredentials(creds), grpc.WithPerRPCCredentials(tokenCreds{&seen}))
	if err != nil {
		panic(err)
	}
	defer cc.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	in, out := []byte("x"), []byte(nil)
	err = cc.Invoke(ctx, "/t.S/Who", &in, &out, grpc.ForceCodecV2(bytesCodec{}))
	if err != nil {
		fmt.Println("who: err " + err.Error())
	} else {
		fmt.Println("who: " + string(out))
	}
	for _, s := range seen {
		fmt.Println("creds: " + strings.ReplaceAll(s, addr, "ADDR"))
	}
}
