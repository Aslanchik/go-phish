package fetcher

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"os/exec"
	"strings"
	"time"
)

const (
	fetcherImage    = "go-phish-fetcher:latest"
	fetchTimeoutSec = "30"
)

// Run fetches the target URL inside a sandboxed container and returns structured results.
// Egress is restricted via an in-process HTTP CONNECT proxy that whitelists only the
// resolved target IPs; the proxy URL is passed to the container via HTTP_PROXY/HTTPS_PROXY.
func Run(ctx context.Context, targetURL string) (FetchResult, error) {
	host, err := extractHost(targetURL)
	if err != nil {
		return FetchResult{}, fmt.Errorf("invalid URL: %w", err)
	}

	ips, err := net.LookupHost(host)
	if err != nil {
		return FetchResult{}, fmt.Errorf("resolve %s: %w", host, err)
	}

	networkName := fmt.Sprintf("gophish-%d", time.Now().UnixNano())
	if _, err := createNetwork(ctx, networkName); err != nil {
		return FetchResult{}, fmt.Errorf("create network: %w", err)
	}
	defer removeNetwork(networkName)

	proxy, err := startProxy(ips)
	if err != nil {
		return FetchResult{}, fmt.Errorf("start proxy: %w", err)
	}
	defer proxy.stop()

	gateway, err := getNetworkGateway(ctx, networkName)
	if err != nil {
		return FetchResult{}, fmt.Errorf("get network gateway: %w", err)
	}

	proxyURL := fmt.Sprintf("http://%s:%d", gateway, proxy.port())
	return runContainer(ctx, targetURL, networkName, proxyURL)
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

func createNetwork(ctx context.Context, name string) (string, error) {
	out, err := exec.CommandContext(ctx, "docker", "network", "create", "--driver", "bridge", name).Output()
	if err != nil {
		return "", fmt.Errorf("docker network create: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

func removeNetwork(name string) {
	exec.Command("docker", "network", "rm", name).Run() //nolint:errcheck
}

func getNetworkGateway(ctx context.Context, networkName string) (string, error) {
	out, err := exec.CommandContext(ctx, "docker", "network", "inspect",
		"--format", "{{(index .IPAM.Config 0).Gateway}}",
		networkName,
	).Output()
	if err != nil {
		return "", fmt.Errorf("docker network inspect: %w", err)
	}
	gw := strings.TrimSpace(string(out))
	if gw == "" {
		return "", fmt.Errorf("no gateway found for network %s", networkName)
	}
	return gw, nil
}

func runContainer(ctx context.Context, targetURL, networkName, proxyURL string) (FetchResult, error) {
	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, "docker", "run",
		"--rm",
		"--cap-drop", "ALL",
		"--security-opt", "no-new-privileges",
		"--read-only",
		"--tmpfs", "/tmp:size=256m",
		"--network", networkName,
		"--env", "TARGET_URL="+targetURL,
		"--env", "FETCH_TIMEOUT_SECONDS="+fetchTimeoutSec,
		"--env", "HTTP_PROXY="+proxyURL,
		"--env", "HTTPS_PROXY="+proxyURL,
		fetcherImage,
	)
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
