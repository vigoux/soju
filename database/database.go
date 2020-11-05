package database

import (
	"fmt"
)

type User struct {
	ID       int64
	Username string
	Password string // hashed
	Admin    bool
}

type SASL struct {
	Mechanism string

	Plain struct {
		Username string
		Password string
	}

	// TLS client certificate authentication.
	External struct {
		// X.509 certificate in DER form.
		CertBlob []byte
		// PKCS#8 private key in DER form.
		PrivKeyBlob []byte
	}
}

type Network struct {
	ID              int64
	Name            string
	Addr            string
	Nick            string
	Username        string
	Realname        string
	Pass            string
	ConnectCommands []string
	SASL            SASL
}

func (net *Network) GetName() string {
	if net.Name != "" {
		return net.Name
	}
	return net.Addr
}

type Channel struct {
	ID       int64
	Name     string
	Key      string
	Detached bool
}

type DB interface {
	Close() error
	ListUsers() ([]User, error)
	GetUser(username string) (*User, error)
	StoreUser(user *User) error
	DeleteUser(id int64) error
	ListNetworks(userID int64) ([]Network, error)
	StoreNetwork(userID int64, network *Network) error
	DeleteNetwork(id int64) error
	ListChannels(networkID int64) ([]Channel, error)
	StoreChannel(networkID int64, ch *Channel) error
	DeleteChannel(id int64) error
}

func Open(driver, source string) (DB, error) {
	switch driver {
	case "sqlite3":
		return openSQLite3(source)
	default:
		return nil, fmt.Errorf("soju/database: unknown database driver %q", driver)
	}
}
