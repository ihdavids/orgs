package common

// Wire types for org-crypt: a heading tagged `:crypt:` whose body is encrypted
// where it sits, in the file, so that what is on disk is ciphertext.
//
// What that is worth, and what it is not, is worth being exact about - because
// an encryption feature that is vague about its threat model is worse than
// none, since people rely on it.
//
// It protects the file: a backup, a sync folder, a git remote, a stolen disk,
// and - just as much - every part of this server that reads org files without
// going through the crypt endpoints. The search index, grep, the link graph,
// every exporter and the starmap only ever see the armoured block, because that
// is all the file holds. Nothing had to be taught to keep a secret; there was no
// secret in what they read.
//
// It does not, on its own, protect against somebody who can reach the server.
// That is a question about who holds the key, and it is why the server holds no
// passphrase: one arrives with the request that needs it, is used, and is
// forgotten. Reaching the server gains an attacker ciphertext.
//
// The strongest arrangement is a public key (`crypt.key`): then the server can
// encrypt with no secret at all and cannot decrypt, whatever is done to it.

// How encryption is done here.
type CryptSettings struct {
	// A gpg key id or fingerprint. Naming one turns on public key mode: the
	// server encrypts to that key and needs no passphrase to do it, and cannot
	// decrypt at all without the private key - which is the arrangement worth
	// having on a server other machines can reach.
	//
	// Empty means symmetric: one passphrase encrypts and decrypts, and it has
	// to be sent with every request that needs it.
	Key string `yaml:"key"`
	// The tag that marks a heading for encryption. "crypt" when empty, which is
	// what org-crypt uses.
	Tag string `yaml:"tag"`
	// The gpg binary. Found on PATH when empty.
	Bin string `yaml:"bin"`
	// How long an unlocked passphrase is kept in memory, in seconds. Zero - the
	// default - keeps none at all, so every request that needs one carries one.
	//
	// Anything above zero is a deliberate trade: it buys not retyping and costs
	// the property that reaching the server gains an attacker only ciphertext.
	// It is a number rather than a switch so that the cost is said out loud.
	UnlockSeconds int `yaml:"unlockSeconds"`
}

// Whether this server can encrypt at all, and how.
type CryptConfig struct {
	Ok  bool
	Msg string
	// off when there is no gpg, symmetric when there is no key, key when there
	// is one. A client draws a very different thing for each.
	Mode string
	// The gpg it found, and what it says about itself.
	Bin     string
	Version string
	// The key it encrypts to, in key mode.
	Key string
	// The tag that marks a heading for encryption.
	Tag string
	// Whether a passphrase is currently held in memory, and for how much longer.
	Unlocked bool
	UnlockIn int
	// How many headings carry the tag, and how many of those are ciphertext
	// right now. The difference is the number of headings that are *meant* to be
	// secret and are sitting in the clear, which is the one number worth putting
	// in front of somebody.
	Tagged    int
	Encrypted int
}

// One heading that carries the tag.
//
// Never its body. A listing is metadata - what is secret, not what the secret
// is - and an endpoint that answered with plaintext because it was convenient
// would undo the whole feature for every client that ever called it.
type CryptHeading struct {
	Hash     string
	Headline string
	Filename string
	Olp      []string
	Line     int
	// Whether the body is ciphertext right now.
	Encrypted bool
	// The key this heading names for itself, from a :CRYPTKEY: property.
	Key string
	// How big the body is, in lines. Enough to say something has been written
	// there without saying what.
	Lines int
}

type CryptHeadings struct {
	Ok       bool
	Msg      string
	Headings []CryptHeading
	// How many are tagged, and how many of those are in the clear.
	Tagged int
	Plain  int
}

// What happened to one heading.
type CryptResult struct {
	Ok  bool
	Msg string

	Hash     string
	Filename string
	// Whether the body is ciphertext after this.
	Encrypted bool
	// The plaintext, and only for a decrypt that was asked to hand it back.
	// Empty everywhere else, including on an encrypt - there is no reason for
	// an encrypt to echo what it has just hidden.
	Text string
}

// What happened to all of them.
type CryptSweepResult struct {
	Ok  bool
	Msg string
	// How many were encrypted, how many were already ciphertext, and how many
	// could not be done - with a line each saying why.
	Encrypted int
	Already   int
	Failed    int
	Problems  []string
	// The files that were written.
	Files []string
}
