package main

import (
	"context"
	"crypto/tls"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestWarmCertificateRetriesAndStopsAfterSuccess(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	calls := 0
	warmCertificate(ctx, "echo.example.com", func(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
		calls++
		require.Equal(t, "echo.example.com", hello.ServerName)
		require.Contains(t, hello.SignatureSchemes, tls.ECDSAWithP256AndSHA256)
		require.Contains(t, hello.SupportedCurves, tls.CurveP256)
		require.Contains(t, hello.CipherSuites, tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256)
		if calls == 1 {
			return nil, errors.New("DNS not ready")
		}
		return &tls.Certificate{}, nil
	}, time.Millisecond)
	require.Equal(t, 2, calls)
}

func TestWarmCertificateStopsOnCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	warmCertificate(ctx, "echo.example.com", func(*tls.ClientHelloInfo) (*tls.Certificate, error) {
		calls++
		cancel()
		return nil, errors.New("unavailable")
	}, time.Hour)
	require.Equal(t, 1, calls)
	warmCertificate(ctx, "echo.example.com", func(*tls.ClientHelloInfo) (*tls.Certificate, error) {
		t.Fatal("canceled warmup must not contact the CA")
		return nil, nil
	}, time.Hour)
}
