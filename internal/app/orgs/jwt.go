package orgs

import (
	"fmt"
	"time"

	"github.com/go-jose/go-jose/v4"

	//"github.com/golang-jwt/jwt/v5"
	"github.com/go-jose/go-jose/v4/jwt"
)

type Claims struct {
	jwt.Claims
	Username string `json:"username"`
	CanWrite bool   `json:"write"`
}

const kORGS_ISSUER = "ORGS"

// Generates a JWS for a user.
func generateToken(username string) (string, error) {
	expirationTime := time.Now().Add(5 * time.Minute)

	claims := &Claims{
		Username: username,
		Claims: jwt.Claims{
			Expiry: jwt.NewNumericDate(expirationTime),
			Issuer: kORGS_ISSUER,
		},
	}

	if signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.HS256, Key: Conf().Server.OrgJWS}, nil); err != nil {
		return "", err
	} else {
		if rawJwt, err := jwt.Signed(signer).Claims(claims).Serialize(); err != nil {
			return "", err
		} else {
			return rawJwt, nil
		}
	}
	//token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	//tokenString, err := token.SignedString(jwtKey)
	//return tokenString, err
}

func encryptJWT(jwtToken string, salt []byte) (string, error) {
	rcpt := jose.Recipient{
		Algorithm:  jose.PBES2_HS256_A128KW,
		Key:        Conf().Server.OrgJWE,
		PBES2Count: 4096,
		PBES2Salt:  []byte(salt),
	}
	if enc, err := jose.NewEncrypter(jose.A128CBC_HS256, rcpt, nil); err != nil {
		return "err", err
	} else {
		if jwePlaintextToken, err := enc.Encrypt([]byte(jwtToken)); err != nil {
			return "err", err
		} else {
			return jwePlaintextToken.FullSerialize(), nil
		}
	}
	/*
		// Do I need this? What does this do?
		if key, err := object.CompactSerialize(); err != nil {
			return "err", err
		}
	*/
}

func decryptJWT(jweToken string) ([]byte, error) {
	if jwe, err := jose.ParseEncrypted(jweToken, []jose.KeyAlgorithm{jose.PBES2_HS256_A128KW}, []jose.ContentEncryption{jose.A128CBC_HS256}); err != nil {
		return nil, err
	} else {
		if decryptedKey, err := jwe.Decrypt(Conf().Server.OrgJWE); err != nil {
			return nil, err
		} else {
			return decryptedKey, nil
		}
	}
}

// After user validation we create a JWT/JWS AND a JWE from that JWS
// that we can then store in a variety of ways and validate that
// the user has authenticated after the fact
func GenerateEncryptedToken(username string) (string, error) {
	if token, err := generateToken(username); err != nil {
		return "err", err
	} else {
		if etoken, err := encryptJWT(token, []byte(Conf().Server.OrgSalt)); err != nil {
			return "err", err
		} else {
			return etoken, nil
		}
	}
}

// Used in pre-login, we pull a cookie or auth header and validate that the users token
// is valid using our keys et al.
func ValidateEncryptedToken(encToken string, claims *Claims) (*jwt.JSONWebToken, error) {
	// First decrypt the encrypted token
	if tokenBytes, err := decryptJWT(encToken); err != nil {
		return nil, err
	} else {
		if rawJwt, err := jwt.ParseSigned(string(tokenBytes), []jose.SignatureAlgorithm{jose.HS256}); err != nil {
			return nil, err
		} else {
			if err := rawJwt.Claims(Conf().Server.OrgJWS, claims); err != nil {
				return nil, err
			} else {
				if err := validateToken(rawJwt, claims); err != nil {
					return nil, err
				} else {
					return rawJwt, nil
				}
			}
		}
	}
}

func validateToken(tkn *jwt.JSONWebToken, claims *Claims) error {
	if claims.Claims.Issuer == kORGS_ISSUER {
		return fmt.Errorf("issuer does not match! Cannot validate claims")
	}
	// TODO: Need more here for validating this user
	if len(claims.Username) < 3 {
		return fmt.Errorf("username does not match expected username")
	}
	if claims.Claims.Expiry.Time().Before(time.Now()) {
		return fmt.Errorf("claim is expired")
	}
	//if claims.RegisteredClaims
	return nil
}

/*
func secureDataHandler(w http.ResponseWriter, r *http.Request) {
	// 1. Extract JWE from Authorization header
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" || len(authHeader) < 7 || authHeader[:7] != "Bearer " {
		http.Error(w, "Unauthorized: Missing or invalid Authorization header", http.StatusUnauthorized)
		return
	}
	jweString := authHeader[7:]

	// 2. Decrypt the JWE
	obj, err := jose.ParseEncrypted(jweString)
	if err != nil {
		http.Error(w, "Unauthorized: Invalid JWE format", http.StatusUnauthorized)
		return
	}

	// Decrypt the JWE using the shared encryption key
	decryptedPayload, err := obj.Decrypt(encryptionKey)
	if err != nil {
		http.Error(w, "Unauthorized: Failed to decrypt JWE", http.StatusUnauthorized)
		return
	}

	// 3. Parse and validate claims
	claims := jwt.Claims{}
	if err := jwt.ParseClaims(decryptedPayload, []jwt.Validator{jwt.AudienceValidator{"your-api-audience"}}, &claims); err != nil {
		http.Error(w, "Unauthorized: Invalid JWE claims", http.StatusUnauthorized)
		return
	}

	// Example: Accessing a custom claim
	var customClaims struct {
		Role string `json:"role"`
	}
	if err := jwt.ParseClaims(decryptedPayload, nil, &customClaims); err != nil {
		http.Error(w, "Error parsing custom claims", http.StatusInternalServerError)
		return
	}

	// 4. Implement authorization logic based on claims
	if customClaims.Role != "admin" {
		http.Error(w, "Forbidden: Insufficient privileges", http.StatusForbidden)
		return
	}

	fmt.Fprintf(w, "Welcome, admin! This is highly confidential data.\n")
}


func generateToken(username string) (string, error) {
	expirationTime := time.Now().Add(5 * time.Minute)
	claims := &Claims{
		Username: username,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expirationTime),
			Issuer:    kORGS_ISSUER,
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString(jwtKey)
	return tokenString, err
}
*/
