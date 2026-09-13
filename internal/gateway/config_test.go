package gateway

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfigUsesLegacyUvicornEnv(t *testing.T) {
	t.Setenv("NEXT_GATEWAY_ADDR", "")
	t.Setenv("UVICORN_HOST", "127.0.0.1")
	t.Setenv("UVICORN_PORT", "9443")
	t.Setenv("UVICORN_SSL_CERTFILE", "/tmp/next/fullchain.pem")
	t.Setenv("UVICORN_SSL_KEYFILE", "/tmp/next/key.pem")

	cfg := LoadConfig()

	if cfg.Addr != "127.0.0.1:9443" {
		t.Fatalf("Addr=%q want %q", cfg.Addr, "127.0.0.1:9443")
	}
	if cfg.TLSCertFile != "/tmp/next/fullchain.pem" {
		t.Fatalf("TLSCertFile=%q", cfg.TLSCertFile)
	}
	if cfg.TLSKeyFile != "/tmp/next/key.pem" {
		t.Fatalf("TLSKeyFile=%q", cfg.TLSKeyFile)
	}
}

func TestLoadConfigKeepsGatewayAddrOverride(t *testing.T) {
	t.Setenv("NEXT_GATEWAY_ADDR", ":18080")
	t.Setenv("UVICORN_HOST", "127.0.0.1")
	t.Setenv("UVICORN_PORT", "9443")

	cfg := LoadConfig()

	if cfg.Addr != ":18080" {
		t.Fatalf("Addr=%q want %q", cfg.Addr, ":18080")
	}
}

func TestLoadConfigReadsNextEnvFile(t *testing.T) {
	envPath := filepath.Join(t.TempDir(), ".env")
	writeTestFile(t, envPath, `
UVICORN_HOST = "127.0.0.1"
UVICORN_PORT = "18083"
UVICORN_SSL_CERTFILE = "/var/lib/next/certs/fullchain.pem"
UVICORN_SSL_KEYFILE = "/var/lib/next/certs/key.pem"
`)
	t.Setenv("NEXT_ENV_FILE", envPath)
	t.Setenv("NEXT_GATEWAY_ADDR", "")
	t.Setenv("UVICORN_HOST", "")
	t.Setenv("UVICORN_PORT", "")
	t.Setenv("UVICORN_SSL_CERTFILE", "")
	t.Setenv("UVICORN_SSL_KEYFILE", "")

	cfg := LoadConfig()

	if cfg.Addr != "127.0.0.1:18083" {
		t.Fatalf("Addr=%q want %q", cfg.Addr, "127.0.0.1:18083")
	}
	if cfg.TLSCertFile != "/var/lib/next/certs/fullchain.pem" {
		t.Fatalf("TLSCertFile=%q", cfg.TLSCertFile)
	}
	if cfg.TLSKeyFile != "/var/lib/next/certs/key.pem" {
		t.Fatalf("TLSKeyFile=%q", cfg.TLSKeyFile)
	}
}

func writeTestFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
