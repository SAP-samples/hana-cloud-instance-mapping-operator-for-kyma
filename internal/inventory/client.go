package inventory

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"golang.org/x/oauth2/clientcredentials"
)

const (
	mappingsPath      = "/inventory/v2/serviceInstances/%s/instanceMappings"
	responseBodyLimit = 65536
)

type Client interface {
	ListMappings(ctx context.Context, serviceInstanceID string) ([]Mapping, error)
	CreateMapping(ctx context.Context, serviceInstanceID string, mapping Mapping) error
	DeleteMapping(ctx context.Context, serviceInstanceID string, primaryID, secondaryID string) error
}

type Mapping struct {
	Platform    string `json:"platform"`
	PrimaryID   string `json:"primaryID"`
	SecondaryID string `json:"secondaryID"`
	IsDefault   bool   `json:"isDefault,omitempty"`
}

const (
	ErrMappingAlreadyExists = inventoryError("mapping already exists")
	ErrMappingNotFound      = inventoryError("mapping not found")
)

type Binding struct {
	BaseURL string
	UAA     BindingUAA
}

type BindingUAA struct {
	URL          string `json:"url,omitempty"`
	ClientID     string `json:"clientid,omitempty"`
	ClientSecret string `json:"clientsecret,omitempty"`
}

type inventoryClient struct {
	Binding Binding
}

func NewClient(binding Binding) Client {
	return &inventoryClient{
		Binding: binding,
	}
}

func (c *inventoryClient) ListMappings(ctx context.Context, serviceInstanceID string) ([]Mapping, error) {
	url := "https://" + c.Binding.BaseURL + fmt.Sprintf(mappingsPath, serviceInstanceID)

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.doAuthRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode == http.StatusOK {
		respReader := http.MaxBytesReader(nil, resp.Body, responseBodyLimit)
		defer respReader.Close()

		respBody := struct {
			Mappings []Mapping `json:"mappings"`
		}{}
		if err := json.NewDecoder(respReader).Decode(&respBody); err != nil {
			return nil, err
		}

		return respBody.Mappings, nil
	}

	return nil, fmt.Errorf("failed to list mappings, HTTP %d", resp.StatusCode)
}

func (c *inventoryClient) CreateMapping(ctx context.Context, serviceInstanceID string, mapping Mapping) error {
	url := "https://" + c.Binding.BaseURL + fmt.Sprintf(mappingsPath, serviceInstanceID)

	bodyBytes := new(bytes.Buffer)
	json.NewEncoder(bodyBytes).Encode(mapping)

	req, err := http.NewRequest(http.MethodPost, url, bodyBytes)
	if err != nil {
		return err
	}

	req.Header.Add("Content-Type", "application/json")

	resp, err := c.doAuthRequest(ctx, req)
	if err != nil {
		return err
	}

	if resp.StatusCode == http.StatusCreated {
		return nil
	}

	if resp.StatusCode == http.StatusOK {
		return ErrMappingAlreadyExists
	}

	return fmt.Errorf("failed to create mapping, HTTP %d", resp.StatusCode)
}

func (c *inventoryClient) DeleteMapping(ctx context.Context, serviceInstanceID string, primaryID, secondaryID string) error {
	url := "https://" + c.Binding.BaseURL + fmt.Sprintf(mappingsPath, serviceInstanceID)

	req, err := http.NewRequest(http.MethodDelete, url, nil)
	if err != nil {
		return err
	}

	values := req.URL.Query()
	values.Add("primaryID", primaryID)
	values.Add("secondaryID", secondaryID)
	req.URL.RawQuery = values.Encode()

	resp, err := c.doAuthRequest(ctx, req)
	if err != nil {
		return err
	}

	if resp.StatusCode == http.StatusOK {
		return nil
	}

	if resp.StatusCode == http.StatusNotFound {
		return ErrMappingNotFound
	}

	return fmt.Errorf("failed to delete mapping, HTTP %d", resp.StatusCode)
}

func (c *inventoryClient) doAuthRequest(ctx context.Context, req *http.Request) (*http.Response, error) {
	config := clientcredentials.Config{
		TokenURL:     c.Binding.UAA.URL + "/oauth/token?grant_type=client_credentials",
		ClientID:     c.Binding.UAA.ClientID,
		ClientSecret: c.Binding.UAA.ClientSecret,
	}

	client := config.Client(ctx)
	return client.Do(req)
}

type inventoryError string

func (e inventoryError) Error() string {
	return string(e)
}
