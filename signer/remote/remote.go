package remote

import (
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/cloudflare/cfssl/api/client"
	"github.com/cloudflare/cfssl/certdb"
	"github.com/cloudflare/cfssl/config"
	cferr "github.com/cloudflare/cfssl/errors"
	"github.com/cloudflare/cfssl/helpers"
	"github.com/cloudflare/cfssl/info"
	"github.com/cloudflare/cfssl/log"
	"github.com/cloudflare/cfssl/signer"
)

// A Signer represents a CFSSL instance running as signing server.
// fulfills the signer.Signer interface
type Signer struct {
	policy      *config.Signing
	reqModifier func(*http.Request, []byte)
	dbAccessor  certdb.Accessor
}

// NewSigner creates a new remote Signer directly from a
// signing policy.
func NewSigner(policy *config.Signing) (*Signer, error) {
	if policy != nil {
		if !policy.Valid() {
			return nil, cferr.New(cferr.PolicyError,
				cferr.InvalidPolicy)
		}
		return &Signer{policy: policy}, nil
	}

	return nil, cferr.New(cferr.PolicyError,
		cferr.InvalidPolicy)
}

// Sign sends a signature request to the remote CFSSL server,
// receiving a signed certificate or an error in response. The hostname,
// csr, and profileName are used as with a local signing operation, and
// the label is used to select a signing root in a multi-root CA.
func (s *Signer) Sign(req signer.SignRequest) (cert []byte, err error) {
	resp, err := s.remoteOp(req, req.Profile, "sign")
	if err != nil {
		return
	}
	if cert, ok := resp.([]byte); ok {

		
		// Get the AKI from signedCert.  This is required to support Go 1.9+.
		// In prior versions of Go, x509.CreateCertificate updated the
		// AuthorityKeyId of certTBS.
		parsedCert, _ := helpers.ParseCertificatePEM(cert)

		// Create JSON req representation for saving in DB.
		var reqJSON []byte
		if reqJSON, err = json.Marshal(req); err != nil {
			return nil, err
		}

		if s.dbAccessor != nil {
			var certRecord = certdb.CertificateRecord{
				Serial:  parsedCert.SerialNumber.String(),
				Subject: parsedCert.Subject.String(),
				// this relies on the specific behavior of x509.CreateCertificate
				// which sets the AuthorityKeyId from the signer's SubjectKeyId
				AKI:       hex.EncodeToString(parsedCert.AuthorityKeyId),
				CALabel:   req.Label,
				CAProfile: req.Profile,
				Status:    "good",
				Expiry:    parsedCert.NotAfter,
				PEM:       string(cert),
                Request:   string(reqJSON),
			}

			err = s.dbAccessor.InsertCertificate(certRecord)
			if err != nil {
				return nil, err
			}

			log.Infof("saved certificate with serial number %s", parsedCert.SerialNumber)
		}
		return cert, nil
	}
	return
}

// Info sends an info request to the remote CFSSL server, receiving an
// Resp struct or an error in response.
func (s *Signer) Info(req info.Req) (resp *info.Resp, err error) {
	respInterface, err := s.remoteOp(req, req.Profile, "info")
	if err != nil {
		return
	}
	if resp, ok := respInterface.(*info.Resp); ok {
		return resp, nil
	}
	return
}

// Helper function to perform a remote sign or info request.
func (s *Signer) remoteOp(req interface{}, profile, target string) (resp interface{}, err error) {
	jsonData, err := json.Marshal(req)
	if err != nil {
		return nil, cferr.Wrap(cferr.APIClientError, cferr.JSONError, err)
	}

	p, err := signer.Profile(s, profile)
	if err != nil {
		return
	}

	server := client.NewServerTLS(p.RemoteServer, helpers.CreateTLSConfig(p.RemoteCAs, p.ClientCert))
	if server == nil {
		return nil, cferr.Wrap(cferr.PolicyError, cferr.InvalidRequest,
			errors.New("failed to connect to remote"))
	}

	server.SetReqModifier(s.reqModifier)

	// There's no auth provider for the "info" method
	if target == "info" {
		resp, err = server.Info(jsonData)
	} else if p.RemoteProvider != nil {
		resp, err = server.AuthSign(jsonData, nil, p.RemoteProvider)
	} else {
		resp, err = server.Sign(jsonData)
	}

	if err != nil {
		return nil, err
	}

	return
}

// SigAlgo returns the RSA signer's signature algorithm.
func (s *Signer) SigAlgo() x509.SignatureAlgorithm {
	// TODO: implement this as a remote info call
	return x509.UnknownSignatureAlgorithm
}

// SetPolicy sets the signer's signature policy.
func (s *Signer) SetPolicy(policy *config.Signing) {
	s.policy = policy
}

// SetDBAccessor sets the signers' cert db accessor, currently noop.
func (s *Signer) SetDBAccessor(dba certdb.Accessor) {
	s.dbAccessor = dba
}

// GetDBAccessor returns the signers' cert db accessor, currently noop.
func (s *Signer) GetDBAccessor() certdb.Accessor {
	return s.dbAccessor
}

// SetReqModifier sets the function to call to modify the HTTP request prior to sending it
func (s *Signer) SetReqModifier(mod func(*http.Request, []byte)) {
	s.reqModifier = mod
}

// Policy returns the signer's policy.
func (s *Signer) Policy() *config.Signing {
	return s.policy
}
