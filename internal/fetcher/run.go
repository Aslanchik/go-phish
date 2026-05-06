package fetcher

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"os/exec"
	"runtime"
	"time"
)

const (
	fetcherImage    = "go-phish-fetcher:latest"
	fetchTimeoutSec = "30"
	// proxyHost is the hostname containers use to reach the host machine.
	// On macOS/Windows, Docker Desktop injects this automatically.
	// On Linux, we pass --add-host=host.docker.internal:host-gateway to docker run.
	proxyHost = "host.docker.internal"
)

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

	networkName := fmt.Sprintf("gophish-%d", time.Now().UnixNano())
	if err := createNetwork(ctx, networkName); err != nil {
		return FetchResult{}, fmt.Errorf("create network: %w", err)
	}
	defer removeNetwork(networkName)

	proxy, err := startProxy(ips)
	if err != nil {
		return FetchResult{}, fmt.Errorf("start proxy: %w", err)
	}
	defer proxy.stop()

	proxyURL := fmt.Sprintf("http://%s:%d", proxyHost, proxy.port())
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
	// On Linux, Docker does not inject host.docker.internal automatically;
	// --add-host maps it to the host gateway so the container can reach the proxy.
	if runtime.GOOS == "linux" {
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
