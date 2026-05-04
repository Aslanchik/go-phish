package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
)

type NetworkEntry struct {
	URL          string `json:"url"`
	Method       string `json:"method"`
	Status       int    `json:"status"`
	ResourceType string `json:"resource_type"`
}

type JSFile struct {
	URL     string `json:"url"`
	Content string `json:"content"`
}

type FormField struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

type Form struct {
	Action string      `json:"action"`
	Method string      `json:"method"`
	Fields []FormField `json:"fields"`
}

type Result struct {
	FinalURL    string         `json:"final_url"`
	RenderedDOM string         `json:"rendered_dom"`
	Screenshot  string         `json:"screenshot"`
	NetworkLog  []NetworkEntry `json:"network_log"`
	JSFiles     []JSFile       `json:"js_files"`
	Forms       []Form         `json:"forms"`
}

func main() {
	log.SetOutput(os.Stderr)
	log.SetFlags(log.Ltime)

	targetURL := os.Getenv("TARGET_URL")
	if targetURL == "" {
		log.Println("TARGET_URL is not set")
		os.Exit(1)
	}

	timeoutSecs := 30
	if s := os.Getenv("FETCH_TIMEOUT_SECONDS"); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil || n <= 0 {
			log.Printf("invalid FETCH_TIMEOUT_SECONDS %q, using default 30", s)
		} else {
			timeoutSecs = n
		}
	}

	result, err := fetch(targetURL, time.Duration(timeoutSecs)*time.Second)
	if err != nil {
		log.Printf("fetch failed: %v", err)
		os.Exit(1)
	}

	if err := json.NewEncoder(os.Stdout).Encode(result); err != nil {
		log.Printf("encode result: %v", err)
		os.Exit(1)
	}
}

func fetch(targetURL string, timeout time.Duration) (*Result, error) {
	l := launcher.New().
		Headless(true).
		NoSandbox(true).
		Leakless(false). // not needed inside a container; avoids exec-from-tmpfs issues
		Set("disable-dev-shm-usage").
		Set("disable-gpu").
		Set("disable-quic").             // prevent UDP bypass of TCP proxy
		Set("disable-dns-over-https")    // force system resolver through proxy

	if bin := os.Getenv("CHROMIUM_BIN"); bin != "" {
		l = l.Bin(bin)
	}

	if proxyURL := os.Getenv("HTTP_PROXY"); proxyURL != "" {
		l = l.Set("proxy-server", proxyURL)
	}

	controlURL, err := l.Launch()
	if err != nil {
		return nil, fmt.Errorf("launch browser: %w", err)
	}

	browser := rod.New().ControlURL(controlURL)
	if err := browser.Connect(); err != nil {
		return nil, fmt.Errorf("connect browser: %w", err)
	}
	defer browser.Close()

	page, err := browser.Timeout(timeout).Page(proto.TargetCreateTarget{URL: "about:blank"})
	if err != nil {
		return nil, fmt.Errorf("create page: %w", err)
	}

	if err := (proto.NetworkEnable{}).Call(page); err != nil {
		return nil, fmt.Errorf("enable network domain: %w", err)
	}

	// Capture network events before navigation so nothing is missed.
	var mu sync.Mutex
	type reqMeta struct {
		method       string
		resourceType proto.NetworkResourceType
	}
	pending := map[proto.NetworkRequestID]reqMeta{}
	var netLog []NetworkEntry
	jsReqURLs := map[proto.NetworkRequestID]string{}

	waitEvents := page.EachEvent(
		func(e *proto.NetworkRequestWillBeSent) bool {
			mu.Lock()
			pending[e.RequestID] = reqMeta{method: e.Request.Method, resourceType: e.Type}
			if e.Type == proto.NetworkResourceTypeScript {
				jsReqURLs[e.RequestID] = e.Request.URL
			}
			mu.Unlock()
			return false
		},
		func(e *proto.NetworkResponseReceived) bool {
			mu.Lock()
			meta := pending[e.RequestID]
			netLog = append(netLog, NetworkEntry{
				URL:          e.Response.URL,
				Method:       meta.method,
				Status:       int(e.Response.Status),
				ResourceType: string(e.Type),
			})
			mu.Unlock()
			return false
		},
	)
	go waitEvents()

	// Set up network-idle wait before navigating so the event is not missed.
	waitIdle := page.WaitNavigation(proto.PageLifecycleEventNameNetworkIdle)

	if err := page.Navigate(targetURL); err != nil {
		return nil, fmt.Errorf("navigate: %w", err)
	}
	waitIdle()

	finalURL, err := page.Eval(`() => location.href`)
	if err != nil {
		return nil, fmt.Errorf("get final URL: %w", err)
	}

	dom, err := page.HTML()
	if err != nil {
		return nil, fmt.Errorf("get HTML: %w", err)
	}

	screenshotBytes, err := page.Screenshot(true, &proto.PageCaptureScreenshot{
		Format: proto.PageCaptureScreenshotFormatPng,
	})
	if err != nil {
		return nil, fmt.Errorf("screenshot: %w", err)
	}

	// Fetch JS file contents from Chrome's response body cache.
	mu.Lock()
	jsIDs := make(map[proto.NetworkRequestID]string, len(jsReqURLs))
	for id, u := range jsReqURLs {
		jsIDs[id] = u
	}
	mu.Unlock()

	jsFiles := []JSFile{}
	for reqID, u := range jsIDs {
		body, err := proto.NetworkGetResponseBody{RequestID: reqID}.Call(page)
		if err != nil {
			log.Printf("JS body unavailable for %s: %v", u, err)
			continue
		}
		content := body.Body
		if body.Base64Encoded {
			if b, err := base64.StdEncoding.DecodeString(body.Body); err == nil {
				content = string(b)
			}
		}
		jsFiles = append(jsFiles, JSFile{URL: u, Content: content})
	}

	forms, err := extractForms(page)
	if err != nil {
		log.Printf("extract forms: %v", err)
		forms = []Form{}
	}

	mu.Lock()
	captured := make([]NetworkEntry, len(netLog))
	copy(captured, netLog)
	mu.Unlock()

	return &Result{
		FinalURL:    finalURL.Value.Str(),
		RenderedDOM: dom,
		Screenshot:  base64.StdEncoding.EncodeToString(screenshotBytes),
		NetworkLog:  captured,
		JSFiles:     jsFiles,
		Forms:       forms,
	}, nil
}

func extractForms(page *rod.Page) ([]Form, error) {
	res, err := page.Eval(`() => Array.from(document.forms).map(f => ({
		action: f.action || "",
		method: (f.method || "get").toLowerCase(),
		fields: Array.from(f.elements)
			.filter(el => el.tagName !== "BUTTON" && el.name)
			.map(el => ({ name: el.name, type: el.type || "text" }))
	}))`)
	if err != nil {
		return nil, fmt.Errorf("eval forms: %w", err)
	}
	var forms []Form
	if err := res.Value.Unmarshal(&forms); err != nil {
		return nil, fmt.Errorf("unmarshal forms: %w", err)
	}
	return forms, nil
}
