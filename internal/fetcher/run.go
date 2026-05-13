package fetcher

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"time"
)

const (
	fetcherImage    = "go-phish-fetcher:latest"
	fetchTimeoutSec = "30"
)

// proxyHost is the hostname fetcher containers use to reach the egress proxy.
// Defaults to host.docker.internal (works for local dev where the server runs on the host).
// Set PROXY_HOST=<service-name> when the server itself runs in a container — the fetcher
// will reach the proxy via Docker DNS on the shared FETCHER_NETWORK.
var proxyHost = envOr("PROXY_HOST", "host.docker.internal")

// fetcherNetwork, when set, is a pre-existing Docker network that the server is already
// attached to. Fetcher containers join it so they can reach the proxy via Docker DNS.
// When empty, a per-investigation bridge network is created and destroyed as before.
var fetcherNetwork = os.Getenv("FETCHER_NETWORK")

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// Run fetches the target URL inside a sandboxed container and returns structured results.
// Egress is restricted via an in-process HTTP CONNECT proxy that whitelists only the
// resolved target IPs; the proxy is reachable from the container via host.docker.internal.
func Run(ctx context.Context, targetURL string) (FetchResult, error) {
	host, err := extractHost(targetURL)
	if err != nil {
		return FetchResult{}, fmt.Errorf("invalid URL: %w", err)
	}

	ips, err := net.LookupHost(host)
	if err != nil {
		return FetchResult{}, fmt.Errorf("resolve %s: %w", host, err)
	}

	proxy, err := startProxy(ips)
	if err != nil {
		return FetchResult{}, fmt.Errorf("start proxy: %w", err)
	}
	defer proxy.stop()

	network := fetcherNetwork
	if network == "" {
		network = fmt.Sprintf("gophish-%d", time.Now().UnixNano())
		if err := createNetwork(ctx, network); err != nil {
			return FetchResult{}, fmt.Errorf("create network: %w", err)
		}
		defer removeNetwork(network)
	}

	proxyURL := fmt.Sprintf("http://%s:%d", proxyHost, proxy.port())
	return runContainer(ctx, targetURL, network, proxyURL)
}

func extractHost(rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	h := u.Hostname()
	if h == "" {
		return "", fmt.Errorf("no host in URL %q", rawURL)
	}
	return h, nil
}

func createNetwork(ctx context.Context, name string) error {
	_, err := exec.CommandContext(ctx, "docker", "network", "create", "--driver", "bridge", name).Output()
	if err != nil {
		return fmt.Errorf("docker network create: %w", err)
	}
	return nil
}

func removeNetwork(name string) {
	exec.Command("docker", "network", "rm", name).Run() //nolint:errcheck
}

func runContainer(ctx context.Context, targetURL, networkName, proxyURL string) (FetchResult, error) {
	args := []string{
		"run", "--rm",
		"--cap-drop", "ALL",
		"--security-opt", "no-new-privileges",
		"--read-only",
		"--tmpfs", "/tmp:size=256m",
		"--network", networkName,
		"--env", "TARGET_URL=" + targetURL,
		"--env", "FETCH_TIMEOUT_SECONDS=" + fetchTimeoutSec,
		"--env", "HTTP_PROXY=" + proxyURL,
		"--env", "HTTPS_PROXY=" + proxyURL,
	}
	// When using a shared named network (FETCHER_NETWORK), the proxy is reachable via
	// Docker DNS (PROXY_HOST = service name) — no host mapping needed.
	// Otherwise on Linux, inject host.docker.internal so the container can reach the
	// host-side proxy via the Docker gateway.
	if fetcherNetwork == "" && runtime.GOOS == "linux" {
		args = append(args, "--add-host=host.docker.internal:host-gateway")
	}
	args = append(args, fetcherImage)

	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return FetchResult{}, fmt.Errorf("container exited non-zero: %w\nstderr: %s", err, stderr.String())
	}

	var result FetchResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		return FetchResult{}, fmt.Errorf("parse container JSON: %w\nraw output: %s", err, stdout.String())
	}
	return result, nil
}
