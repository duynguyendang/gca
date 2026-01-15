package meb

import "github.com/dgraph-io/badger/v4"

// withReadTxn executes a function within a read transaction.
func (m *MEBStore) withReadTxn(fn func(*badger.Txn) error) error {
	if m.txn != nil {
		// Inside a batch, reuse existing transaction
		return fn(m.txn)
	}
	txn := m.newTxn()
	defer m.releaseTxn(txn)
	return fn(txn)
}

// withWriteTxn executes a function within a write transaction.
func (m *MEBStore) withWriteTxn(fn func(*badger.Txn) error) error {
	if m.txn != nil {
		// Inside a batch, reuse existing transaction
		// Note: We assume the batch transaction is writable (ExecuteBatch uses Update)
		return fn(m.txn)
	}
	txn := m.db.NewTransaction(true)
	defer txn.Discard()
	if err := fn(txn); err != nil {
		return err
	}
	return txn.Commit()
}
