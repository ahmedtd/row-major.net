package main

import (
	"context"
	"flag"
	"log"
	"os"
	"syscall"

	"row-major/cloud-checkout/pkg/storage"
	ccproto "row-major/cloud-checkout/proto"

	"github.com/dgraph-io/badger"
	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
	"golang.org/x/xerrors"
)

type FUSENode struct {
	// Must embed an Inode for the struct to work as a node.
	fs.Inode

	storage *storage.Storage
}

// Ensure we are implementing the NodeReaddirer interface
var _ = (fs.NodeReaddirer)((*FUSENode)(nil))

// Readdir is part of the NodeReaddirer interface
func (n *FUSENode) Readdir(ctx context.Context) (fs.DirStream, syscall.Errno) {
	childFSEntries := map[string]*ccproto.FSNode{}
	err := n.storage.DB.View(func(txn *badger.Txn) error {
		childFSEntries, err := n.storage.Readdir(txn, n.StableAttr().Ino)
		if err != nil {
			return err
		}
	})
	if err != nil {
		log.Printf("Failure in Readdir(): %v", err)
		serr := &storage.Error{}
		if xerrors.As(err, &serr) {
			return nil, serr.Errno
		}
		return nil, syscall.EIO
	}

	r := make([]fuse.DirEntry, 0, len(childFSEntries))
	for name, child := range childFSEntries {
		childMode := uint32(0)
		if child.GetFile().GetPresent() {
			childMode = fuse.S_IFREG
		} else if child.GetFile().GetPresent() {
			childMode = fuse.S_IFDIR
		} else {
			return nil, syscall.EBADF
		}

		d := fuse.DirEntry{
			Name: name,
			Ino:  childFSNode.GetId(),
			Mode: childMode,
		}
		r = append(r, d)
	}
	return fs.NewListDirStream(r), 0
}

// Ensure we are implementing the NodeLookuper interface
var _ = (fs.NodeLookuper)((*FUSENode)(nil))

// Lookup is part of the NodeLookuper interface
func (n *FUSENode) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	childFSEntries := []*ccproto.FSNode{}
	err := n.storage.DB.View(func(txn *badger.Txn) error {
		dirEntry, err := n.storage.GetFSNode(txn, n.StableAttr().Ino)
		if err != nil {
			return xerrors.Errorf("while retrieving parent node: %w", err)
		}

		childFSEntries, err = n.storage.GetFSNodeChildren(txn, dirEntry)
		if err != nil {
			return xerrors.Errorf("while retrieving child nodes: %w", err)
		}
		return nil
	})
	if err != nil {
		log.Printf("Failure in Lookup(): %v", err)
		serr := &storage.Error{}
		if xerrors.As(err, &serr) {
			return nil, serr.Errno
		}
		return nil, syscall.EIO
	}

	selectedChildFSNode := (*ccproto.FSNode)(nil)
	for _, childFSNode := range childFSEntries {
		if childFSNode.GetName() == name {
			selectedChildFSNode = childFSNode
		}
	}
	if selectedChildFSNode == nil {
		return nil, syscall.ENOENT // Didn't have a child with the name we're looking for.
	}

	selectedChildMode := uint32(0)
	if selectedChildFSNode.GetFile().GetPresent() {
		selectedChildMode = fuse.S_IFREG
	} else if selectedChildFSNode.GetDirectory().GetPresent() {
		selectedChildMode = fuse.S_IFDIR
	} else {
		return nil, syscall.EIO // The child is neither file nor directory.  Storage bug or corruption.
	}

	stable := fs.StableAttr{
		Mode: selectedChildMode,
		Ino:  selectedChildFSNode.GetId(),
	}
	operations := &FUSENode{
		storage: n.storage,
	}

	// The NewInode call wraps the `operations` object into an Inode.
	child := n.NewInode(ctx, operations, stable)

	// In case of concurrent lookup requests, it can happen that operations !=
	// child.Operations().
	return child, 0
}

var _ = (fs.NodeCreater)((*FUSENode)(nil))

func (n *FUSENode) Create(ctx context.Context, name string, flags uint32, mode uint32, out *fuse.EntryOut) (node *fs.Inode, fh fs.FileHandle, fuseFlags uint32, errno syscall.Errno) {

	childFSNode := &ccproto.FSNode{}
CommitRetry:
	err := n.storage.DB.Update(func(txn *badger.Txn) error {
		parentFSNode, err := n.storage.GetFSNode(txn, n.StableAttr().Ino)
		if err != nil {
			return xerrors.Errorf("while retrieving parent node from kv-store: %w", err)
		}

		childFSNode = &ccproto.FSNode{
			Name: name,
		}

		if mode&fuse.S_IFREG != 0 {
			childFSNode.File = &ccproto.FSNode_File{
				Present: true,
			}
		} else if mode&fuse.S_IFDIR != 0 {
			childFSNode.Directory = &ccproto.FSNode_Directory{
				Present:  true,
				Children: []uint64{},
			}
		}

		return n.storage.CreateChild(txn, parentFSNode, childFSNode)
	})
	if xerrors.Is(err, badger.ErrConflict) {
		log.Printf("Transaction conflict in Create(), retrying")
		goto CommitRetry
	} else if err != nil {
		log.Printf("Failure in Create(): %v", err)
		serr := &storage.Error{}
		if xerrors.As(err, &serr) {
			return nil, nil, 0, serr.Errno
		}
		return nil, nil, 0, syscall.EIO
	}

	// Right now, the only modes we handle in the storage layer are file vs
	// directory.  Clip the provided mode down so it only records those
	// attributes, to avoid a confusing mismatch between the in-memory and
	// on-disk states of the filesystem.
	selectedMode := mode & (fuse.S_IFREG | fuse.S_IFDIR)

	stable := fs.StableAttr{
		Mode: selectedMode,
		Ino:  childFSNode.GetId(),
	}
	operations := &FUSENode{
		storage: n.storage,
	}

	// The NewInode call wraps the `operations` object into an Inode.
	child := n.NewInode(ctx, operations, stable)

	// In case of concurrent lookup requests, it can happen that operations !=
	// child.Operations().
	return child, nil, 0, 0
}

var (
	mountDir  = flag.String("mount-dir", "/tmp/cloud-checkout", "Mount point")
	dataDir   = flag.String("data-dir", "/tmp/cloud-checkout-kv", "Storage directory")
	clearData = flag.Bool("clear-data", false, "Clear storage directory before opening")
)

func main() {
	flag.Parse()

	storage, err := storage.New(*dataDir, *clearData)
	if err != nil {
		log.Fatal(err)
	}
	defer storage.Close()

	// This is where we'll mount the FS
	os.Mkdir(*mountDir, 0755)
	root := &FUSENode{
		storage: storage,
	}
	server, err := fs.Mount(*mountDir, root, &fs.Options{
		MountOptions: fuse.MountOptions{
			// Set to true to see how the file system works.
			Debug: true,
		},
	})
	if err != nil {
		log.Panic(err)
	}

	log.Printf("Mounted on %s", *mountDir)
	log.Printf("Unmount by calling 'fusermount -u %s'", *mountDir)

	// Wait until unmount before exiting
	server.Wait()
}
