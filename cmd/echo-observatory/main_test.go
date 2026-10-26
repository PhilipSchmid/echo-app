package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/PhilipSchmid/echo-observatory/v2/internal/config"
	"github.com/sirupsen/logrus"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

func TestFlagDefaultsMatchConfiguration(t *testing.T) {
	previous := pflag.CommandLine
	pflag.CommandLine = pflag.NewFlagSet("defaults", pflag.ContinueOnError)
	viper.Reset()
	t.Cleanup(func() { pflag.CommandLine = previous; viper.Reset() })
	defineFlags()
	_, err := config.Load()
	require.NoError(t, err)
	// Do not bind flags: compare the CLI defaults with the independent loader defaults.
	pflag.CommandLine.VisitAll(func(flag *pflag.Flag) {
		t.Run(flag.Name, func(t *testing.T) {
			require.Equal(t, flag.DefValue, fmt.Sprint(viper.Get(flag.Name)))
		})
	})
}

func captureCertificateLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	logger := logrus.StandardLogger()
	output, level := logger.Out, logger.GetLevel()
	var logs bytes.Buffer
	logger.SetOutput(&logs)
	logger.SetLevel(logrus.DebugLevel)
	t.Cleanup(func() { logger.SetOutput(output); logger.SetLevel(level) })
	return &logs
}

func TestWarmCertificateLogsDoNotExposeCAErrorContents(t *testing.T) {
	logs := captureCertificateLogs(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	calls := 0
	warmCertificate(ctx, "echo.example.com", func(*tls.ClientHelloInfo) (*tls.Certificate, error) {
		calls++
		if calls == 1 {
			return nil, errors.New("private-marker\nforged-log-line")
		}
		return &tls.Certificate{}, nil
	}, time.Millisecond)
	require.Equal(t, 2, calls)
	require.Contains(t, logs.String(), "ACME certificate not ready; retrying")
	require.Contains(t, logs.String(), "echo.example.com")
	require.NotContains(t, logs.String(), "private-marker")
	require.NotContains(t, logs.String(), "forged-log-line")
}

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
	logs := captureCertificateLogs(t)
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	warmCertificate(ctx, "echo.example.com", func(*tls.ClientHelloInfo) (*tls.Certificate, error) {
		calls++
		cancel()
		return nil, errors.New("unavailable")
	}, time.Hour)
	require.Equal(t, 1, calls)
	require.Empty(t, logs.String(), "normal shutdown must not emit a certificate warning")
	warmCertificate(ctx, "echo.example.com", func(*tls.ClientHelloInfo) (*tls.Certificate, error) {
		t.Fatal("canceled warmup must not contact the CA")
		return nil, nil
	}, time.Hour)
}
