package main

import (
	"encoding/json"
	"fmt"
)

/*
keys.go — dev-only keystore convenience over the node admin RPC (:50003). Lets demo/verify setup
create and read BLS keys without raw curl. NOTE: a real frontend manages keys client-side and never
talks to the node keystore; this is purely local dev ergonomics.
*/

// KeystoreNew creates a new key in the node keystore and returns its address (hex).
func (c *Client) KeystoreNew(nickname, password string) (string, error) {
	rb, err := postJSON(c.AdminRPC+"/v1/admin/keystore-new-key",
		fmt.Sprintf(`{"nickname":%q,"password":%q}`, nickname, password))
	if err != nil {
		return "", err
	}
	var addr string
	if err := json.Unmarshal(rb, &addr); err != nil {
		return "", fmt.Errorf("parse keystore-new-key %q: %w", string(rb), err)
	}
	return addr, nil
}

// keyGroup mirrors the admin RPC keystore-get response (Go-cased field names).
type keyGroup struct {
	Address    string `json:"Address"`
	PublicKey  string `json:"PublicKey"`
	PrivateKey string `json:"PrivateKey"`
}

// KeystoreGet returns the key material for an address from the node keystore.
func (c *Client) KeystoreGet(address, password string) (keyGroup, error) {
	var kg keyGroup
	rb, err := postJSON(c.AdminRPC+"/v1/admin/keystore-get",
		fmt.Sprintf(`{"address":%q,"password":%q}`, address, password))
	if err != nil {
		return kg, err
	}
	if err := json.Unmarshal(rb, &kg); err != nil {
		return kg, fmt.Errorf("parse keystore-get %q: %w", string(rb), err)
	}
	return kg, nil
}
