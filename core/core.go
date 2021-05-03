package core

import (
	"math"
	"sync"
)

type Wendy struct {
	validators []Validator
	quorum     int // quorum gets updated every time the validator set is updated.

	txsMtx sync.RWMutex
	txs    map[Hash]Tx

	votesMtx sync.RWMutex
	votes    map[Hash]*Vote
	senders  map[ID]*Sender
}

func New() *Wendy {
	return &Wendy{
		txs:     make(map[Hash]Tx),
		votes:   make(map[Hash]*Vote),
		senders: make(map[ID]*Sender),
	}
}

// UpdateValidatorSet updates the list of validators in the consensus.
// Updating the validator set might affect the return value of Quorum().
// Upon updating the senders that are not in the new validator set are removed.
func (w *Wendy) UpdateValidatorSet(vs []Validator) {
	w.validators = vs

	q := math.Floor(
		float64(len(vs))*Quorum,
	) + 1
	w.quorum = int(q)

	w.votesMtx.Lock()
	defer w.votesMtx.Unlock()
	senders := make(map[ID]*Sender)
	// keep all the senders we already have and create new one if not present
	// those old senders that are not part of the new set will be discarded.
	for _, v := range vs {
		key := ID(v)
		if s, ok := w.senders[key]; ok {
			senders[key] = s
		} else {
			senders[key] = NewSender(key)
		}
	}
	w.senders = senders
}

// HonestParties returns the required number of votes to be sure that at least
// one vote came from a honest validator.
// t + 1
func (w *Wendy) HonestParties() int {
	return w.quorum
}

// HonestMajority returns the minimum number of votes required to assure that I
// have a honest majority (2t + 1, which is equivalent to n-t). It's also the maximum number of honest parties I can
// expect to have.
func (w *Wendy) HonestMajority() int {
	return len(w.validators) - w.quorum
}

// AddTx adds a tx to the list of tx to be mined.
// AddTx returns false if the tx was already added.
// NOTE: This function is safe for concurrent access.
func (w *Wendy) AddTx(tx Tx) bool {
	w.txsMtx.Lock()
	defer w.txsMtx.Unlock()

	hash := tx.Hash()
	if _, ok := w.txs[hash]; ok {
		return false
	}
	w.txs[hash] = tx
	return true
}

// AddVote adds a vote to the list of votes.
// Votes are positioned given it's sequence number.
// AddVote returns alse if the vote was already added.
// NOTE: This function is safe for concurrent access.
func (w *Wendy) AddVote(v *Vote) bool {
	w.votesMtx.Lock()
	defer w.votesMtx.Unlock()

	// Register the vote on the sender
	sender, ok := w.senders[v.Pubkey]
	if !ok {
		sender = NewSender(v.Pubkey)
		w.senders[v.Pubkey] = sender
	}

	// Register the vote based on its tx.Hash
	w.votes[v.TxHash] = v

	return sender.AddVote(v)
}

// CommitBlock iterate over the block's Txs set and remove them from Wendy's
// internal state.
// Txs present on block were probbaly added in the past via AddTx().
func (w *Wendy) CommitBlock(block Block) {
	w.votesMtx.Lock()
	defer w.votesMtx.Unlock()

	for _, sender := range w.senders {
		sender.UpdateTxSet(block.Txs...)
	}
}

// VoteByTxHash returns a vote given its tx.Hash
// Returns nil if the vote hasn't been seen.
// NOTE: This function is safe for concurrent access.
func (w *Wendy) VoteByTxHash(hash Hash) *Vote {
	w.votesMtx.RLock()
	defer w.votesMtx.RUnlock()
	return w.votes[hash]
}

// hasQuorum evaluates fn for every register sender.
// It returns true if fn returned true at least w.Quorum() times.
// NOTE: This function is safe for concurrent access.
func (w *Wendy) hasQuorum(fn func(s *Sender) bool) bool {
	w.votesMtx.RLock()
	defer w.votesMtx.RUnlock()

	var votes int
	for _, s := range w.senders {
		if ok := fn(s); ok {
			votes++
			if votes == w.quorum {
				return true
			}
		}
	}
	return false
}

// IsBlockedBy determines if tx2 might have priority over tx1.
// We say that tx1 is NOT bloked by tx2 if there are t+1 votes reporting tx1
// before tx2.
func (w *Wendy) IsBlockedBy(tx1, tx2 Tx) bool {
	// if there's no quorum that tx1 is before tx2, then tx1 is Blocked by tx2
	return !w.hasQuorum(func(s *Sender) bool {
		return s.Before(tx1, tx2)
	})
}

// IsBlocked identifies if it is pssible that a so-far-unknown transaction
// might be scheduled with priority to tx.
func (w *Wendy) IsBlocked(tx Tx) bool {
	// if there's no quorum that tx has been seen, then IsBlocked
	return !w.hasQuorum(func(s *Sender) bool {
		return s.Seen(tx)
	})
}
