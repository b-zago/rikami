package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/bitnami-labs/sealed-secrets/pkg/kubeseal"
)

type Tokens struct {
	Short   string `json:"token_short"`
	Refresh string `json:"token_refresh"`
}

type ReqUserLogin struct {
	User     string
	Password string
}

type Response struct {
	Message string `json:"message"`
	Tokens
}

func APILogin(c *Config, reqUser *ReqUserLogin) *Response {
	client := &http.Client{Timeout: 10 * time.Second}
	url := c.URL + "/login"
	reqBody := parseRequest(reqUser)
	sig := c.ComputeHMAC(reqBody)

	req, err := http.NewRequest("POST", url, bytes.NewReader(reqBody))
	apiErr(err)
	req.Header.Set("x-rikami-signature", base64.RawStdEncoding.EncodeToString(sig))

	resp, err := client.Do(req)
	apiErr(err)
	defer resp.Body.Close()

	return readResponse(resp)
}

func APICert(c *Config) *rsa.PublicKey {
	client := &http.Client{Timeout: 10 * time.Second}
	url := c.URL + "/cert"
	resp, err := client.Get(url)
	if err != nil {
		log.Fatalf("could not fetch cert from %q %v", url, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Fatalf("could not read response on /cert %v", err)
	}

	r := parseResponse(body, resp.StatusCode)
	if resp.StatusCode != http.StatusOK {
		log.Fatalf("error on /cert endpoint: %s", r.Message)
	}

	cert, err := kubeseal.ParseKey(strings.NewReader(r.Message))
	if err != nil {
		log.Fatalf("could not parse sealed-secrets public key. error: %v", err)
	}
	return cert
}

func (c *Config) ComputeHMAC(b []byte) []byte {
	mac := hmac.New(sha256.New, []byte(c.HMAC))
	mac.Write(b)
	return mac.Sum(nil)
}

func readResponse(resp *http.Response) *Response {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Fatalf("could not read response on %q %v", resp.Request.URL.Path, err)
	}
	r := parseResponse(body, resp.StatusCode)
	if resp.StatusCode != http.StatusOK {
		if r.Message == "" {
			r.Message = "unknown server error"
		}
		log.Fatalf("error on %q endpoint: %s", resp.Request.URL.Path, r.Message)
	}
	return r
}

func parseResponse(b []byte, status int) *Response {
	var res Response
	err := json.Unmarshal(b, &res)
	if err != nil {
		log.Fatalf("could not parse server JSON response: %v\nstatus: %d", err, status)
	}
	return &res
}

func parseRequest(req any) []byte {
	b, err := json.Marshal(req)
	if err != nil {
		log.Fatalf("could not parse client JSON request: %v", err)
	}
	return b
}

func apiErr(err error) {
	if err != nil {
		log.Fatalf("api error: %v", err)
	}
}
