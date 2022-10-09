package storage

import (
	"encoding/binary"
	"fmt"
	"log"
	"os"
	"syscall"

	ccproto "row-major/cloud-checkout/proto"

	"github.com/dgraph-io/badger"
	"github.com/golang/protobuf/proto"
	"golang.org/x/xerrors"
)

// Key prefixes that denote different tables in the key-value store.
//
// See the readme for documentation on the tables themselves.
const (
	KeyTypeRevision      uint32 = 0
	KeyTypeRevisionIDSeq uint32 = 1
	KeyTypeFSNode        uint32 = 2
	KeyTypeFSNodeIDSeq   uint32 = 3
	KeyTypeInodeIndex    uint32 = 4
	KeyTypeInodeSeq      uint32 = 5
	KeyTypeNodeToInode   uint32 = 6
)

func RevisionKey(revisionID uint64) []byte {
	key := make([]byte, 12)
	binary.BigEndian.PutUint32(key[0:4], KeyTypeRevision)
	binary.BigEndian.PutUint64(key[4:12], revisionID)
	return key
}

func RevisionKeyPrefixAllRevision() []byte {
	key := make([]byte, 4)
	binary.BidEndian.PutUint32(key[0:4], KeyTypeRevision)
	return key
}

func FSNodeKey(fsnode_id uint64, revision_id uint64) []byte {
	key := make([]byte, 20)
	binary.BigEndian.PutUint32(key[0:4], KeyTypeFSNode)
	binary.BigEndian.PutUint64(key[4:12], fsnode_id)
	binary.BigEndian.PutUint64(key[12:20], revision_id)
	return key
}

func DecodeFSNodeKey(key []byte) (fsNodeID uint64, revisionID uint64, err error) {
	if len(key) != 20 {
		return xerrors.New("key has wrong length; got %d, want 20", len(key))
	}
	fsNodeID = binary.BigEndian.Uint64(key[4:12])
	revisionID = binary.BigEndian.Uint64(key[12:20])
	return fsNodeID, revisionID, nil
}

func FSNodeKeyPrefixAllFSNodeAllRevision() []byte {
	key := make([]byte, 4)
	binary.BigEndian.PutUint32(key[0:4], KeyTypeFSNode)
	return key
}

func FSNodeKeyPrefixOneFSNodeAllRevision(fsnode_id uint64) []byte {
	key := make([]byte, 12)
	binary.BigEndian.PutUint32(key[0:4], KeyTypeFSNode)
	binary.BigEndian.PutUint64(key[4:12], fsnode_id)
	return key
}

func FSNodeIDSeqKey() []byte {
	key := make([]byte, 4)
	binary.BigEndian.PutUint32(key[0:4], KeyTypeFSNodeIDSeq)
	return key
}

func InodeIndexKey(inode uint64) []byte {
	key := make([]byte, 12)
	binary.BigEndian.PutUint32(key[0:4], KeyTypeInodeIndex)
	binary.BigEndian.PutUint64(key[4:12], inode)
	return key
}

func InodeSeqKey() []byte {
	key := make([]byte, 4)
	binary.BigEndian.PutUint32(key[0:4], KeyTypeInodeSeq)
	return key
}

type Error struct {
	Errno   syscall.Errno
	Message string

	inner error
	frame xerrors.Frame
}

func NewError(errno syscall.Errno, message string, inner error) *Error {
	return &Error{
		Errno:   errno,
		Message: message,
		inner:   inner,
		frame:   xerrors.Caller(1),
	}
}

func (e *Error) Error() string {
	return fmt.Sprintf("%s (errno %q): %v", e.Message, e.Errno, e.inner)
}

func (e *Error) Format(f fmt.State, c rune) { // implements fmt.Formatter
	xerrors.FormatError(e, f, c)
}

func (e *Error) FormatError(p xerrors.Printer) error { // implements xerrors.Formatter
	p.Print(fmt.Sprintf("%s (errno %q)", e.Message, e.Errno))
	if p.Detail() {
		e.frame.Format(p)
	}
	return nil
}

func (e *Error) Unwrap() error {
	return e.inner
}

type Storage struct {
	DB *badger.DB

	revisionIDSeq *badger.Sequence
	fsNodeIDSeq   *badger.Sequence
	inodeIDSeq    *badger.Sequence
}

func New(dataDir string, clear bool) (*Storage, error) {
	if clear {
		if err := os.RemoveAll(dataDir); err != nil {
			return nil, xerrors.Errorf("while clearing data dir %q: %w", dataDir, err)
		}
	}

	// Open the Badger database located in the /tmp/cloud-checkout-badger directory.
	// It will be created if it doesn't exist.
	db, err := badger.Open(badger.DefaultOptions(dataDir))
	if err != nil {
		return nil, xerrors.Errorf("while opening badger kv dir: %w", err)
	}

	revisionIDSeq, err := db.GetSequence(RevisionIDSeqKey(), 100)
	if err != nil {
		return nil, xerrors.Errorf("while retrieving Revision ID sequence: %w", err)
	}

	fsNodeIDSeq, err := db.GetSequence(FSNodeIDSeqKey(), 100)
	if err != nil {
		return nil, xerrors.Errorf("while retrieving FSNode ID sequence: %w", err)
	}

	inodeIDSeq, err := db.GetSequence(InodeIDSeqKey(), 100)
	if err != nil {
		return nil, xerrors.Errorf("while retrieving inode sequence: %w", err)
	}

	storage := &Storage{
		DB:            db,
		revisionIDSeq: revisionIDSeq,
		fsNodeIDSeq:   fsNodeIDSeq,
		inodeIDSeq:    inodeIDSeq,
	}

	// Write a root node into our database
CommitRetry:
	err = storage.DB.Update(func(txn *badger.Txn) error {
		_, err := storage.EnsureRoot(txn)
		return err
	})
	if xerrors.Is(err, badger.ErrConflict) {
		goto CommitRetry
	} else if err != nil {
		log.Fatalf("while creating root node: %v", err)
	}

	return storage, nil
}

func (s *Storage) Close() error {
	if err := s.revisionIDSeq.Release(); err != nil {
		return xerrors.Errorf("while releasing Revision ID sequence: %w", err)
	}

	if err := s.fsNodeIDSeq.Release(); err != nil {
		return xerrors.Errorf("while releasing FSNode ID sequence: %w", err)
	}

	if err := s.inodeIDSeq.Release(); err != nil {
		return xerrors.Errorf("while releasing inode ID sequence: %w", err)
	}

	if err := s.DB.Close(); err != nil {
		return xerrors.Errorf("while closing database: %w", err)
	}

	return nil
}

// GetLatestRevision retrieves the latest revision from the revision table.
func (s *Storage) GetLatestRevision(txn *badger.Txn) (*ccproto.Revision, error) {
	it := txn.NewIterator(&badger.IteratorOptions{
		Prefetch: true,
		Prefetch: 1,
		Reverse:  true,
		Prefix:   RevisionKeyPrefixAllRevision(),
	})

	for it.Rewind(); it.Valid(); it.Next() {
		item := it.Item()

		curRevID, err := DecodeRevisionKey(it.KeyCopy())
		if err != nil {
			return nil, NewError(syscall.EIO, "while decoding Revision key ", err)
		}

		rev := &ccproto.Revision{}
		if err := proto.Unmarshal(it.ValueCopy(), rev); err != nil {
			return nil, NewError(syscall.EIO, fmt.Sprintf("while unmarshalling Revision %d", curRevID), err)
		}

		if rev.GetRevisionId() != curRevID {
			return nil, NewError(syscall.EIO, fmt.Sprintf("inconsistency between Revision key and value, key is for revision ID %d, but value has revision ID %d", curRevID, rev.GetRevisionID), nil)
		}

		return rev, nil
	}

	return nil, NewError(syscall.EIO, "found no revisions", nil)
}

// GetFSNodeAsOfRevision retrieves the state of the given FSNode at the provided
// revision.
//
// It operates by looking backwards through the FSNode revision table until it
// finds a version of the given FSNode with revision less than or equal to the
// given revision ID.
func (s *Storage) GetFSNodeAsOfRevision(txn *badger.Txn, fsNodeID uint64, revID uint64) (*ccproto.FSNode, error) {
	it := txn.NewIterator(&badger.IteratorOptions{
		Prefetch: true,
		Prefetch: 100,
		Reverse:  true,
		Prefix:   FSNodeKeyPrefixOneFSNodeAllRevision(fsnode_id),
	})

	for it.Rewind(); it.Valid(); it.Next() {
		item := it.Item()

		curFSNodeID, curRevID, err := DecodeFSNodeKey(it.KeyCopy())
		if err != nil {
			return nil, NewError(syscall.EIO, "while decoding FSNode key", err)
		}

		if curRevID <= revID {
			fsNode := &ccproto.FSNode{}
			if err := proto.Unmarshal(item.ValueCopy(), fsNode); err != nil {
				return nil, NewError(syscall.EIO, fmt.Sprintf("while unmarshalling revision of FSNode %d at revision %d", fsNodeID, curRevID), err)
			}

			if fsNode.GetFSNodeID() != curFSNodeID {
				return nil, NewError(syscall.EIO, fmt.Sprintf("inconsistency between FSNode key and value, key has ID %d, but value has ID %d", curFSNodeID, fsNode.GetFSNodeID()), nil)
			}

			if fsNode.GetRevisionID() != curRevID {
				return nil, NewError(syscall.EIO, fmt.Sprintf("inconsistency between FSNode key and value, key has revision ID %d, but value has revision ID %d", curRevID, fsNode.GetRevisionID()), nil)
			}

			return fsNode, nil
		}
	}

	if !it.Valid() {
		return nil, NewError(syscall.EIO, fmt.Sprintf("found no revision of FSNode %d before %d", fsNodeID, revID), nil)
	}
}

func (s *Storage) targetRevForInode(txn *badger.Txn, inode *ccproto.InodeIndexEntry) (targetRev uint64, err error) {
	switch inodeInfo.GetMode() {
	case ccproto.InodeIndexEntry_Mode_Live:
		revID, err := s.GetLatestRevision(txn)
		if err != nil {
			return nil, xerrors.Errorf("while retrieving latest revision: %w", err)
		}
		return revID, nil
	case ccproto.InodeIndexEntry_Mode_Snapshot:
		return inodeInfo.GetRevisionId(), nil
	}
}

func (s *Storage) getInodeIndexEntry(txn *badger.Txn, inode uint64) (*ccproto.InodeIndexEntry, error) {
	inodeIndexItem, err := txn.Get(InodeIndexKey(inode))
	if err != nil {
		if xerrors.Is(err, badger.ErrKeyNotFound) {
			return nil, NewError(syscall.EINVAL, fmt.Sprintf("inode %d doesn't exist in kv-store", inode), err)
		}
		return nil, NewError(syscall.EIO, fmt.Sprintf("failure while looking up inode %d", inode), err)
	}

	inodeInfo := &ccproto.InodeIndexEntry{}
	if err := proto.Unmarshal(inodeIndexItem.ValueCopy(), inodeIndexEntry); err != nil {
		return nil, NewError(syscall.EIO, fmt.Sprintf("while unmarshaling inode index entry for inode %d", inode), err)
	}

	return inodeInfo, nil
}

// GetFSNodeForInode looks up the the correct filesystem entry for the given inode.
//
// It works by first looking up the inode in the InodeIndex table, then
// following the link to the correct FSNode table entry.
func (s *Storage) GetFSNodeForInode(txn *badger.Txn, inode uint64) (*ccproto.FSNode, error) {
	inodeInfo, err := s.getInodeIndexEntry(txn, inode)
	if err != nil {
		return nil, xerrors.Errorf("while looking up inode index entry for inode %d: %w", inode, err)
	}

	targetRev, err := s.getTargetRevForInode(txn, inodeInfo)
	if err != nil {
		return nil, xerrors.Errorf("while determining target revision for inode %d: %w", inode, err)
	}

	fsNode, err := s.GetFSNodeAsOfRevision(txn, inodeInfo.GetFSNodeId(), targetRev)
	if err != nil {
		return nil, xerrors.Errorf("while retrieving FSNode %d at revision %d: %w", inodeInfo.GetFsNodeId(), revID, err)
	}

	return fsNode, nil
}

type ReaddirResult struct {
	name  string
	info  *ccproto.FSNode
	inode uint64
}

func (s *Storage) Readdir(txn *badger.Txn, inode uint64) (map[string]*ccproto.FSNode, error) {
	inodeInfo, err := s.getInodeIndexEntry(txn, inode)
	if err != nil {
		return nil, xerrors.Errorf("while looking up inode index entry for inode %d: %w", inode, err)
	}

	targetRev, err := s.getTargetRevForInode(txn, inodeInfo)
	if err != nil {
		return nil, xerrors.Errorf("while determining target revision for inode %d: %w", inode, err)
	}

	parent, err := s.GetFSNodeAsOfRevision(txn, inodeInfo.GetFSNodeId(), targetRev)
	if err != nil {
		return nil, xerrors.Errorf("while retrieving FSNode %d at revision %d: %w", inodeInfo.GetFsNodeId(), revID, err)
	}

	if !parent.GetDirectory().GetPresent() {
		return nil, NewError(syscall.EINVAL, fmt.Sprintf("parent (inode %d) (name %q) is not a directory", parent.GetId(), parent.GetName()), nil)
	}

	children := make(map[string]*ccproto.FSNode, 0, len(parent.GetDirectory().GetChildren()))
	for _, entry := range parent.GetDirectory().GetChildren() {
		child, err := s.GetFSNodeAsOfRevision(txn, entry.GetFsNodeId(), targetRev)
		if err != nil {
			return nil, err
		}

		// TODO: Introduce new inode index entries for child if one doesn't already exist.

		children[entry.GetName()] = child
	}

	return children, nil
}

func (s *Storage) EnsureRoot(txn *badger.Txn) (*ccproto.FSNode, error) {
	root, err := s.GetFSNode(txn, 1)
	if xerrors.Is(err, badger.ErrKeyNotFound) {
		goto MakeRoot
	}
	if err != nil {
		return nil, xerrors.Errorf("while looking up root FSNode: %w", err)
	}
MakeRoot:

	root = &ccproto.FSNode{
		Directory: &ccproto.FSNode_Directory{
			Present: true,
		},
	}

	// The root of the filesystem needs to have inode 1, so skip ID 0.
	skipID, err := s.fsNodeIDSeq.Next()
	if err != nil {
		return nil, NewError(syscall.EIO, "couldn't advance fsNodeIDSeq", err)
	}
	log.Printf("Skipped id %d", skipID)

	if err := s.CreateFSNode(txn, root); err != nil {
		return nil, xerrors.Errorf("while recording root FSNode: %w", err)
	}

	root, err = s.GetFSNode(txn, 1)
	if xerrors.Is(err, badger.ErrKeyNotFound) {
		return nil, NewError(
			syscall.EIO, // This should not be EINVAL, but EIO.  Something is seriously wrong.
			"couldn't find root FSNode that we just wrote",
			err,
		)
	}
	if err != nil {
		return nil, xerrors.Errorf("while looking up root FSNode: %w", err)
	}

	return root, nil
}

func (s *Storage) CreateFSNode(txn *badger.Txn, node *ccproto.FSNode) error {
	id, err := s.fsNodeIDSeq.Next()
	if err != nil {
		return NewError(syscall.EIO, "failure getting next FSNode ID", err)
	}

	log.Printf("Creating node with id %d", id)

	node.Id = id

	return s.SetFSNode(txn, node)
}

func (s *Storage) SetFSNode(txn *badger.Txn, node *ccproto.FSNode) error {
	if node.GetFile().GetPresent() {
	} else if node.GetDirectory().GetPresent() {
		// TODO: Sanity check provided child ids
	} else {
		return NewError(syscall.EIO, "provided node is neither file nor directory", nil)
	}

	nodeBytes, err := proto.Marshal(node)
	if err != nil {
		return NewError(syscall.EIO, "failure marshalling FSNode", err)
	}

	if err := txn.Set(FSNodeKey(node.Id), nodeBytes); err != nil {
		return NewError(syscall.EIO, "failure recording FSNode in kv-store", err)
	}

	return nil
}

func (s *Storage) CreateChild(txn *badger.Txn, parent *ccproto.FSNode, child *ccproto.FSNode) error {
	if !parent.GetDirectory().GetPresent() {
		return NewError(syscall.EIO, "parent node is not a directory", nil)
	}

	children, err := s.GetFSNodeChildren(txn, parent)
	if err != nil {
		return xerrors.Errorf("while retrieving existing children of parent: %w", err)
	}

	for _, ec := range children {
		if ec.GetName() == child.GetName() {
			return NewError(syscall.EEXIST, fmt.Sprintf("parent node already has child named %q", child.GetName()), nil)
		}
	}

	err = s.CreateFSNode(txn, child) // Fills out ID of child
	if err != nil {
		return xerrors.Errorf("while recording child node: %w", err)
	}

	parent.GetDirectory().Children = append(parent.GetDirectory().GetChildren(), child.GetId())

	err = s.SetFSNode(txn, parent)
	if err != nil {
		return xerrors.Errorf("while recording updated parent: %w", err)
	}

	return nil
}

// AssertConsistent performs a global consistency check.
//
// It checks the following invariants:
//
//   * There is exactly one node
//
//   *
// func (s *Storage) AssertConsistent(txn *badger.Txn) error {
//	it := txn.NewIterator(&badger.IteratorOptions{
//		Prefetch:     true,
//		PrefetchSize: 1000,
//		Prefix:       FSNodeKeyPrefix(),
//	})

//	for it.Rewind(); it.Valid(); it.Next() {
//		item := it.Item()

//		fsEntry := &ccproto.FSNode{}
//		if err := proto.Unmarshal(item.ValueCopy(), fsEntry); err != nil {
//			return xerrors.Errorf("while unmarshalling value: %w", err)
//		}

//	}
// }
