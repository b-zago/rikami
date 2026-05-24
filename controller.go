package main

import (
	"bufio"
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

type EnvEntry struct {
	EnvName string
	EnvVals map[string]string
}

type SummonRequest struct {
	Vessel     string
	LibVersion string
	Name       string
	Envs       []EnvEntry
}

type AppRequest struct {
	Action  string
	Pattern string
	Param   string
}

type Response struct {
	Message string
}

var httpClient = &http.Client{
	Timeout: 30 * time.Second,
}

func ForgeRequest(vsl string, envs []EnvEntry) *SummonRequest {
	scanner := bufio.NewScanner(os.Stdin)
	fmt.Print("Library version: ")
	scanner.Scan()
	lib := scanner.Text()
	fmt.Print("Chart name: ")
	scanner.Scan()
	name := scanner.Text()

	return NewSummonReq(vsl, lib, name, envs)
}

func NewSummonReq(vsl string, libVer string, name string, envs []EnvEntry) *SummonRequest {
	return &SummonRequest{
		Vessel:     vsl,
		LibVersion: libVer,
		Name:       name,
		Envs:       envs,
	}
}

func NewAppRequest(action, pattern, param string) *AppRequest {
	return &AppRequest{
		Action:  action,
		Pattern: pattern,
		Param:   param,
	}
}

func (r SummonRequest) Send() error {
	respBody, err := doSignedRequest(http.MethodPost, "/summon", r)
	if err != nil {
		return err
	}

	var envs []EnvEntry
	if err := json.Unmarshal(respBody, &envs); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	for _, env := range envs {
		fmt.Printf("Env: %s\n", env.EnvName)
		for k, v := range env.EnvVals {
			fmt.Printf("  %s=%s\n", k, v)
		}
	}
	return nil
}

func (r AppRequest) Send() error {
	respBody, err := doSignedRequest(http.MethodPost, "/app", r)
	if err != nil {
		return err
	}

	var resp Response

	if err := json.Unmarshal(respBody, &resp); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	fmt.Println("Response:", resp.Message)

	return nil
}

func doSignedRequest(method, path string, payload any) ([]byte, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	url := Config.URL + path

	mac := hmac.New(sha256.New, []byte(Config.Token))
	mac.Write(body)
	signature := base64.StdEncoding.EncodeToString(mac.Sum(nil))

	req, err := http.NewRequest(method, url, bytes.NewBuffer(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "HMAC "+signature)
	req.Header.Set("X-User-ID", Config.UserID)

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(respBody))
	}
	return respBody, nil
}
