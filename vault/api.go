package vault

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/spf13/viper"
)

// Secrets represents Vault key/value pairs for a prefix
type Secrets struct {
	KVPairs map[string]string `json:"data"`
}

// Read displays all key/value pairs at the given prefix if no key is given
// If a key is passed will just display the key/value pair for the key
func Read(url string) (*Secrets, error) {
	s, err := readSecrets(url)
	if err != nil {
		return nil, err
	}
	if len(s.KVPairs) == 0 {
		return nil, fmt.Errorf("no secrets yet")
	}
	return s, nil
}

// Write sets the given key/value pairs at the provided prefix
func Write(url string, kvs []string, s *Secrets) (*Secrets, error) {
	for _, kvp := range kvs {
		p := strings.Split(kvp, "=")
		for k, v := range s.KVPairs {
			if k == p[0] {
				fmt.Printf("changing %s from %s => %s\n", k, v, p[1])
			}
		}
	}

	for _, kvp := range kvs {
		p := strings.Split(kvp, "=")
		s.KVPairs[p[0]] = p[1]
	}

	if err := updateSecrets(url, s); err != nil {
		return nil, err
	}

	return s, nil
}

// Delete removes a key/value pair from the prefix
func Delete(url string, keys []string, s *Secrets) (*Secrets, error) {
	var count int
	for _, key := range keys {
		for k := range s.KVPairs {
			if k == key {
				delete(s.KVPairs, k)
			}
			count = len(s.KVPairs)
		}
		if len(s.KVPairs) != count {
			fmt.Printf("deleted key %s\n", key)
		}
	}
	if err := updateSecrets(url, s); err != nil {
		return nil, err
	}

	return s, nil
}

func readSecrets(url string) (*Secrets, error) {
	client := &http.Client{}
	req, _ := http.NewRequest("GET", url, strings.NewReader(""))
	req.Header.Set("X-Vault-Token", viper.GetString("vault_token"))
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("failed to reach vault: %s\n%s", resp.Status, string(b))
	}
	s := &Secrets{}
	if err := json.NewDecoder(resp.Body).Decode(s); err != nil {
		return nil, err
	}
	return s, nil
}

func updateSecrets(url string, s *Secrets) error {
	j, err := json.Marshal(s.KVPairs)
	if err != nil {
		return err
	}
	client := &http.Client{}
	req, _ := http.NewRequest("POST", url, bytes.NewReader(j))
	req.Header.Set("X-Vault-Token", viper.GetString("vault_token"))
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		b, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return err
		}
		return fmt.Errorf("failed to set vault secrets: %s\n%s", resp.Status, string(b))
	}
	return nil
}

// SecretsURL returns the Vault API endpoint to GET and POST secrets
// for a given app and env
func SecretsURL(app, env string) string {
	vaultHost := viper.GetString("vault_host")
	return fmt.Sprintf("https://%s/v1/%s", vaultHost, prefix(app, env))
}

func prefix(app, env string) string {
	return fmt.Sprintf("secret/%s/%s", app, env)
}