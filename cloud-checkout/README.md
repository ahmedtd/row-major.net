The idea is to have something like CitC for my personal use.  I often
work on a project from several machines, and it's annoying to try to
sync my work between them.  I have to introduce dummy commits and then
sync them up.

A CitC workalike, perhaps with basic offline support, would let me
sidestep that problem.  My working state would be synced between
machines automatically.

It will need to have some verion-control awareness.

The current idea is: a FUSE driver talks over GRPC to a central server
that keeps the filesystem state in a key-value store (trying out
Badger).  This gives flexibility for implementing snapshots and undo.

Right now, I'm prototyping by having the FUSE driver just read and
write directly to the Badger KV store.

## KV Layout

```
revision modified_nodes
       0 0:Dir()                      // Initial revision.  One empty directory added at
                                      //   the root.
       1 0:Dir(a=1)     1:File        // Add a new file named `a` to the directory
       2 0:Dir(a=1,b=2)        2:File // Add another new file named `b` to the directory
       3                1:File        // Modification to file 1 --- mtime modification,
                                      //   mode modification, content modification, etc.
       4 0:Dir(b=2)                   // Delete file 1 from the directory.  It can still
                                      //  be linked from other directories.
```

The KV store is split into separate tables using prefix tags (assigned
below).  The tag is a uint32.  Each key is a fixed-format binary
message, composed by concatenating the fixed-width binary
representations of each key field.  In order to optimize the ability
to scan meaningful ranges of keys, each key field is stored in
big-endian format.

### Revision Table (tag = 0)

The Revision table stores Revision protobufs, keyed by `(uint32 tag=0,
uint64 revision_id)` tuples.

### RevisionIDSeq (tag = 1)

The RevisionIDSeq table stores a badger sequence holding the next
available Revision ID.

### FSNode Table (tag = 2)

The FSNode table stores FSNode protobufs, keyed by `(uint32 tag=1,
uint64 fsnode_id, uint64 revision_id)` tuples.

### FSNodeIDSeq Table (tag = 3)

The FSNodeIDSeq table stores a badger sequence holding the next
available FSNode ID.

### InodeIndex Table (tag = 4)

The InodeIndex table stores InodeIndexEntry protobufs, keyed by
`(uint32 tag = 4, uint64 inode)` tuples.  It maps inodes being used by
the FUSE client to the FSNodes that they refer to.

### InodeSeq table (tag = 5)

The InodeSeq table stores a badger sequence holding the next available
inode value.  It needs to start from 2, since 0 and 1 have special
meanings to the FUSE library.  0 means the FUSE library should
allocate a new inode number, and 1 is the fixed value for the mount
point.

I think it's safe to share inode numbers between clients.

### NodeToInode table (tag = 6)

The NodeToInode table stores empty values, keyed by `(uint32 tag = 6,
uint64 fsnode_id, uint32 mode, uint64 target_revision, uint64 inode)`
tuples.  It is used when we have just discovered new FSNodes (for
example, during Readdir on a snapshot) and need to check if they
already have inodes.

If mode is Live, then the only valid value for target_revision is 0.

## Testing:

bazel run //cmd/cloud-checkout-fuse-client -- --clear-data=true
touch /tmp/cloud-checkout/x
fusermount -u /tmp/cloud-checkout