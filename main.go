package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	svchost "github.com/hashicorp/terraform-svchost"
	"github.com/pkg/errors"

	"github.com/hashicorp/vault/api"
	"github.com/hashicorp/vault/command/token"
)

var GitCommit = ""
var Version = "0.0.0"
var PreRelease = "dev"

func main() {

	vaultBasePath := flag.String("vault-path", os.Getenv("TF_TOKEN_VAULT_PATH"), "base kv2 path to search")
	flag.Parse()

	if *vaultBasePath == "" {
		fmt.Fprintf(os.Stderr, "Error: --vault-path or TF_TOKEN_VAULT_PATH not set.\n\n")
		usage()
	}

	values := flag.Args()

	if len(values) < 2 {
		usage()
	}

	switch values[0] {
	case "get":
		var err error
		// The credentials helper protocol calls for Terraform to provide the
		// hostname already in the "for comparison" form, so we'll assume that
		// here and let this not match if the caller isn't behaving.
		hostname := values[1]
		wantedHost := svchost.Hostname(hostname)

		fmt.Fprintf(os.Stderr, "Reading secret from Vault at %s\n", *vaultBasePath)
		secret, err := readSecretFromVault(*vaultBasePath)
		fmt.Fprintf(os.Stderr, "secret %s\n", secret)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Unable to read secret from Vault: %s", err)
			os.Exit(1)
		}

		creds, err := generateTokenMap(hostname, secret)
		if err != nil {
			fmt.Fprintf(os.Stderr, "unable to generate %s", err)
			os.Exit(1)
		}

		token, ok := creds[wantedHost]
		if !ok {
			// No stored credentials for a host is a non-error case; respond
			// with an empty credentials object.
			os.Stdout.WriteString("{}\n")
			os.Exit(0)
		}
		result := resultJSON{token}
		resultJSON, err := json.Marshal(result)
		if err != nil {
			// Should never happen
			fmt.Fprintf(os.Stderr, "Failed to serialize result: %s\n", err)
			os.Exit(1)
		}
		os.Stdout.Write(resultJSON)
		os.Stdout.WriteString("\n")
		os.Exit(0)

	default:
		fmt.Fprintf(os.Stderr, "The 'vault' credentials helper is not able to %s credentials.\n", values[0])
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "Usage: terraform-credentials-vault --vault-path <base secrets kv path> get <hostname>")
	fmt.Fprintln(os.Stderr, "\nThis is a Terraform credentials helper, not intended to be run directly from a shell.")
	os.Exit(1)
}

func readSecretFromVault(secretPath string) (string, error) {

	config := api.DefaultConfig()

	if err := config.ReadEnvironment(); err != nil {
		return "", errors.Wrap(err, "failed to read environment")
	}
	client, err := api.NewClient(config)
	if err != nil {
		return "", errors.Wrap(err, "failed to create client")
	}

	// Get the token if it came in from the environment
	clientToken := client.Token()

	// If we don't have a token, check the token helper
	if clientToken == "" {
		helper, err := token.NewInternalTokenHelper()
		if err != nil {
			return "", errors.Wrap(err, "failed to get token helper")
		}
		clientToken, err = helper.Get()
		if err != nil {
			return "", errors.Wrap(err, "failed to get token from token helper")
		}
	}

	// Set the token
	if clientToken != "" {
		client.SetToken(clientToken)
	} else {
		return "", errors.Wrap(err, "failed to get token from environment or credential helper")
	}
	secret, err := client.Logical().Read(secretPath)
	if err != nil {
		fmt.Println("in if err == nil")
		return "", errors.Wrap(err, fmt.Sprintf("failed to read secret from Vault at %s\n", secretPath))
	}

	// dont error if we can read a secret for a particular provider such as the Terraform registry
	if secret == nil {
		return "", nil
	}

	m := secret.Data

	s := fmt.Sprintf("%v", m["token"])

	if s == "" {
		return "", fmt.Errorf("no secret data at =%s does not contain a token attribute or its empty", secretPath)
	}

	return s, nil
}

func generateTokenMap(hostname string, token string) (map[svchost.Hostname]string, error) {

	ret := make(map[svchost.Hostname]string)
	dispHost := svchost.ForDisplay(hostname)
	wantedHost, err := svchost.ForComparison(dispHost)
	if err != nil {
		return nil, errors.Wrap(err, "failed to convert hostname")
	}

	ret[wantedHost] = token

	return ret, nil
}

type resultJSON struct {
	Token string `json:"token"`
}
