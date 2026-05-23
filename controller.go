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
)

type EnvEntry struct {
	EnvName string
	EnvVals map[string]string
}

type RequestSummon struct {
	Vessel     string
	LibVersion string
	Name       string
	Envs       []EnvEntry
}

func ForgeRequest(vsl string, envs []EnvEntry) *RequestSummon {
	scanner := bufio.NewScanner(os.Stdin)
	fmt.Print("Library version: ")
	scanner.Scan()
	lib := scanner.Text()
	fmt.Print("Chart name: ")
	scanner.Scan()
	name := scanner.Text()

	return NewSummonReq(vsl, lib, name, envs)
}

func NewSummonReq(vsl string, libVer string, name string, envs []EnvEntry) *RequestSummon {
	return &RequestSummon{
		Vessel:     vsl,
		LibVersion: libVer,
		Name:       name,
		Envs:       envs,
	}
}

func (r RequestSummon) Send() error {
	body, err := json.Marshal(r)
	if err != nil {
		return err
	}
	url := Config.URL + "/summon"

	mac := hmac.New(sha256.New, []byte(Config.Token))
	mac.Write(body)
	signature := base64.StdEncoding.EncodeToString(mac.Sum(nil))

	req, err := http.NewRequest(http.MethodPost, url, bytes.NewBuffer(body))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "HMAC "+signature)
	req.Header.Set("X-User-ID", Config.UserID)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(respBody))
	}

	var envs []EnvEntry
	if err := json.NewDecoder(resp.Body).Decode(&envs); err != nil {
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
