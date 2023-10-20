package jwtverify

import (
	"crypto/ecdsa"
	"crypto/rsa"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"

	"github.com/THPTUHA/kairos/server/deliverer/jwks"
	"github.com/THPTUHA/kairos/server/deliverer/rule"
	"github.com/cristalhq/jwt/v5"
	"github.com/rs/zerolog/log"
)

type jwksManager struct{ *jwks.Manager }

type VerifierConfig struct {
	// HMACSecretKey is a secret key used to validate connection and subscription
	// tokens generated using HMAC. Zero value means that HMAC tokens won't be allowed.
	HMACSecretKey string

	// RSAPublicKey is a public key used to validate connection and subscription
	// tokens generated using RSA. Zero value means that RSA tokens won't be allowed.
	RSAPublicKey *rsa.PublicKey

	// ECDSAPublicKey is a public key used to validate connection and subscription
	// tokens generated using ECDSA. Zero value means that ECDSA tokens won't be allowed.
	ECDSAPublicKey *ecdsa.PublicKey

	// JWKSPublicEndpoint is a public url used to validate connection and subscription
	// tokens generated using rotating RSA public keys. Zero value means that JSON Web Key Sets
	// extension won't be used.
	JWKSPublicEndpoint string

	// Audience when set will enable audience token check. See
	// https://datatracker.ietf.org/doc/html/rfc7519#section-4.1.3.
	Audience string

	// AudienceRegex allows setting Audience in form of Go language regex pattern. Regex groups
	// may be then used in constructing JWKSPublicEndpoint.
	AudienceRegex string

	// Issuer when set will enable a check that token issuer matches configured string.
	// See https://datatracker.ietf.org/doc/html/rfc7519#section-4.1.1.
	Issuer string

	// IssuerRegex allows setting Issuer in form of Go language regex pattern. Regex groups
	// may be then used in constructing JWKSPublicEndpoint.
	IssuerRegex string
}

func (c VerifierConfig) Validate() error {
	if c.Audience != "" && c.AudienceRegex != "" {
		return errors.New("can not use both token_audience and token_audience_regex, configure only one of them")
	}
	if c.Issuer != "" && c.IssuerRegex != "" {
		return errors.New("can not use both token_issuer and token_issuer_regex, configure only one of them")
	}
	return nil
}

func NewTokenVerifierJWT(config VerifierConfig, ruleContainer *rule.Container) (*VerifierJWT, error) {
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("error validating token verifier config: %w", err)
	}
	var audienceRe *regexp.Regexp
	var issuerRe *regexp.Regexp
	var err error
	if config.AudienceRegex != "" {
		audienceRe, err = regexp.Compile(config.AudienceRegex)
		if err != nil {
			return nil, fmt.Errorf("error compiling audience regex: %w", err)
		}
	}
	if config.IssuerRegex != "" {
		issuerRe, err = regexp.Compile(config.IssuerRegex)
		if err != nil {
			return nil, fmt.Errorf("error compiling issuer regex: %w", err)
		}
	}

	verifier := &VerifierJWT{
		ruleContainer: ruleContainer,
		issuer:        config.Issuer,
		issuerRe:      issuerRe,
		audience:      config.Audience,
		audienceRe:    audienceRe,
	}

	algorithms, err := newAlgorithms(config.HMACSecretKey, config.RSAPublicKey, config.ECDSAPublicKey)
	if err != nil {
		return nil, fmt.Errorf("error initializing token algorithms: %w", err)
	}
	verifier.algorithms = algorithms

	if config.JWKSPublicEndpoint != "" {
		mng, err := jwks.NewManager(config.JWKSPublicEndpoint)
		if err != nil {
			return nil, fmt.Errorf("error creating JWK manager: %w", err)
		}
		verifier.jwksManager = &jwksManager{mng}
	}

	return verifier, nil
}

type algorithms struct {
	HS256 jwt.Verifier
	HS384 jwt.Verifier
	HS512 jwt.Verifier
	RS256 jwt.Verifier
	RS384 jwt.Verifier
	RS512 jwt.Verifier
	ES256 jwt.Verifier
	ES384 jwt.Verifier
	ES512 jwt.Verifier
}

type VerifierJWT struct {
	mu            sync.RWMutex
	jwksManager   *jwksManager
	algorithms    *algorithms
	ruleContainer *rule.Container
	audience      string
	audienceRe    *regexp.Regexp
	issuer        string
	issuerRe      *regexp.Regexp
}

func newAlgorithms(tokenHMACSecretKey string, rsaPubKey *rsa.PublicKey, ecdsaPubKey *ecdsa.PublicKey) (*algorithms, error) {
	alg := &algorithms{}

	var algorithms []string

	// HMAC SHA.
	if tokenHMACSecretKey != "" {
		verifierHS256, err := jwt.NewVerifierHS(jwt.HS256, []byte(tokenHMACSecretKey))
		if err != nil {
			return nil, err
		}
		verifierHS384, err := jwt.NewVerifierHS(jwt.HS384, []byte(tokenHMACSecretKey))
		if err != nil {
			return nil, err
		}
		verifierHS512, err := jwt.NewVerifierHS(jwt.HS512, []byte(tokenHMACSecretKey))
		if err != nil {
			return nil, err
		}
		alg.HS256 = verifierHS256
		alg.HS384 = verifierHS384
		alg.HS512 = verifierHS512
		algorithms = append(algorithms, []string{"HS256", "HS384", "HS512"}...)
	}

	// RSA.
	if rsaPubKey != nil {
		if verifierRS256, err := jwt.NewVerifierRS(jwt.RS256, rsaPubKey); err != nil {
			if err != jwt.ErrInvalidKey {
				return nil, err
			}
		} else {
			alg.RS256 = verifierRS256
			algorithms = append(algorithms, "RS256")
		}
		if verifierRS384, err := jwt.NewVerifierRS(jwt.RS384, rsaPubKey); err != nil {
			if err != jwt.ErrInvalidKey {
				return nil, err
			}
		} else {
			alg.RS384 = verifierRS384
			algorithms = append(algorithms, "RS384")
		}
		if verifierRS512, err := jwt.NewVerifierRS(jwt.RS512, rsaPubKey); err != nil {
			if err != jwt.ErrInvalidKey {
				return nil, err
			}
		} else {
			alg.RS512 = verifierRS512
			algorithms = append(algorithms, "RS512")
		}
	}

	// ECDSA.
	if ecdsaPubKey != nil {
		if verifierES256, err := jwt.NewVerifierES(jwt.ES256, ecdsaPubKey); err != nil {
			if err != jwt.ErrInvalidKey {
				return nil, err
			}
		} else {
			alg.ES256 = verifierES256
			algorithms = append(algorithms, "ES256")
		}
		if verifierES384, err := jwt.NewVerifierES(jwt.ES384, ecdsaPubKey); err != nil {
			if err != jwt.ErrInvalidKey {
				return nil, err
			}
		} else {
			alg.ES384 = verifierES384
			algorithms = append(algorithms, "ES384")
		}
		if verifierES512, err := jwt.NewVerifierES(jwt.ES512, ecdsaPubKey); err != nil {
			if err != jwt.ErrInvalidKey {
				return nil, err
			}
		} else {
			alg.ES512 = verifierES512
			algorithms = append(algorithms, "ES512")
		}
	}

	if len(algorithms) > 0 {
		log.Info().Str("algorithms", strings.Join(algorithms, ", ")).Msg("enabled JWT verifiers")
	}

	return alg, nil
}
