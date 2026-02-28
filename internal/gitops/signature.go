package gitops

import (
	"time"

	"github.com/go-git/go-git/v5/plumbing/object"
)

// botSignature returns the commit author used for automated gitops commits.
func botSignature() *object.Signature {
	return &object.Signature{
		Name:  "sikifanso",
		Email: "sikifanso@local",
		When:  time.Now(),
	}
}
