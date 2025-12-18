package tls

import (
	"crypto/tls"
)

type Store struct {
	certs map[string]*tls.Certificate
}

func NewStore() *Store {
	return &Store{certs: make(map[string]*tls.Certificate)}
}

func (s *Store) Add(domain string, cert *tls.Certificate) {
	s.certs[domain] = cert
}

func (s *Store) Get(domain string) (*tls.Certificate, bool) {
	cert, ok := s.certs[domain]
	return cert, ok
}
