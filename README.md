# VortexTorrent (`vortex-torrent`)

[![Go](https://img.shields.io/badge/Go-1.27+-00ADD8?style=flat&logo=go)](https://go.dev/)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Build Status](https://img.shields.io/badge/build-passing-brightgreen.svg)]()
[![Tests](https://img.shields.io/badge/tests-all%20passing-success.svg)]()
[![Dependencies](https://img.shields.io/badge/dependencies-zero%20(std%20only)-blueviolet.svg)]()

A high-performance, concurrent **BitTorrent P2P client and network engine** written from scratch in pure Go. Built strictly according to the official BitTorrent protocol specifications (BEP 0003) and inspired by Jesse Li's canonical BitTorrent architecture.

VortexTorrent implements a custom **Recursive Descent Bencode Parser**, **Tracker HTTP Announce Client**, **Peer Wire Protocol TCP State Machine**, and a **Concurrent Multi-Peer Downloader** utilizing Go's native goroutines and channels.

---

## Architectural Overview

```text
                               +-----------------------------+
                               |     .torrent Metafile       |
                               +--------------+--------------+
                                              |
                                              v
                               +-----------------------------+
                               |   Custom Bencode Decoder    |
                               |  - Lexical Tokenizer        |
                               |  - Raw Byte Slicing for     |
                               |    20-Byte SHA-1 InfoHash   |
                               +--------------+--------------+
                                              |
                                              v
                               +-----------------------------+
                               |    HTTP Tracker Announce    |
                               |  - Binary info_hash query   |
                               |  - Compact 6-Byte Peer List |
                               +--------------+--------------+
                                              |
                                              v
                             [Swarm Peers: IP_1, IP_2, ... IP_N]
                                              |
                     +------------------------+------------------------+
                     |                        |                        |
                     v                        v                        v
          +--------------------+   +--------------------+   +--------------------+
          | Peer Worker #1     |   | Peer Worker #2     |   | Peer Worker #N     |
          | - 68B TCP Handshake|   | - 68B TCP Handshake|   | - 68B TCP Handshake|
          | - Bitfield / State |   | - Bitfield / State |   | - Bitfield / State |
          | - 16KB Pipelining  |   | - 16KB Pipelining  |   | - 16KB Pipelining  |
          +----------+---------+   +----------+---------+   +----------+---------+
                     \                        |                        /
                      \ (workQueue chan)      | (results chan)        /
                       +----------------------+----------------------+
                                              |
                                              v
                               +-----------------------------+
                               |    File Assembler & Store   |
                               |  - SHA-1 Piece Verification |
                               |  - Multi-Piece Disk Writer  |
                               +-----------------------------+
```

---

## Core Technical Highlights

### 1. Pure Standard Library (Zero External Dependencies)
Built entirely using Go's built-in standard packages (`net`, `net/http`, `crypto/sha1`, `encoding/binary`). The `go.mod` file contains **zero third-party dependencies**, ensuring absolute supply-chain security and minimal binary footprint.

### 2. Zero-Copy Raw Bencode Slicing for `info_hash`
The BitTorrent specification mandates that the 20-byte `info_hash` must be computed by SHA-1 hashing the exact raw bencoded bytes of the `info` dictionary. Rather than deserializing and re-serializing (which can alter key order or formatting), VortexTorrent's decoder records byte offsets during recursive traversal to extract the exact slice directly:

$$\text{InfoHash} = \text{SHA-1}(\text{RawBencodeBytes}[\text{valStart} : \text{valEnd}])$$

### 3. Pipelined 16KB Block Requests
To maximize network throughput over high-latency TCP connections, each peer worker pipelines up to **5 unacknowledged 16KB block requests** (`MaxBacklog = 5`) concurrently, keeping the network pipe full rather than waiting for stop-and-wait round-trips.

### 4. Cryptographic Piece Verification
Every assembled piece is verified against its corresponding 20-byte SHA-1 hash from the `.torrent` file before being written to disk. If a piece fails verification (e.g. due to bit rot or malicious peer injection), it is immediately rejected, quarantined, and re-queued for download from an alternate peer.

---

## Package Architecture

| Package | Responsibility |
| :--- | :--- |
| `bencode/` | Recursive descent parser decoding strings, integers, lists, and dictionaries with raw byte slicing. |
| `torrentfile/` | High-level `.torrent` parser, 20-byte SHA-1 `info_hash` calculator, and HTTP Tracker announce client. |
| `peers/` | Compact 6-byte peer format unpacker (`[4B IPv4][2B BigEndian Port]`). |
| `message/` | 68-byte BitTorrent handshake serializer and wire protocol message codecs (Choke, Unchoke, Have, Bitfield, Request, Piece). |
| `bitfield/` | Thread-safe bit-array tracking piece availability across peers. |
| `client/` | TCP connection wrapper managing handshake verification, keep-alives, and unchoke state transitions. |
| `p2p/` | Orchestrator managing concurrent worker goroutines, work distribution queue, and file assembly. |

---

## Quickstart & CLI Guide

### Building and Running Tests
Ensure Go 1.22+ is installed:

```powershell
# Clone the repository
git clone https://github.com/gxmngi/vortex-torrent.git
cd vortex-torrent

# Run the complete test suite (includes mock peer server tests)
go test -v ./...

# Build the executable binary
go build -o vortex-torrent.exe main.go
```

---

### CLI Commands

```powershell
# 1. Inspect torrent metadata and 20-byte SHA-1 InfoHash
.\vortex-torrent.exe info ubuntu.torrent

# 2. Query public tracker and list live peer endpoints in the swarm
.\vortex-torrent.exe peers ubuntu.torrent

# 3. Perform 68-byte TCP handshake with a live peer
.\vortex-torrent.exe handshake ubuntu.torrent

# 4. Concurrently download file from swarm peers with SHA-1 validation
.\vortex-torrent.exe download ubuntu.torrent ubuntu.iso
```

---

### Real-World Example Output

```text
==================================================
 VortexTorrent Engine v0.1.0 (Go Native)
 Concurrent BitTorrent P2P Downloader
==================================================
Tracker URL  : https://torrent.ubuntu.com/announce
File Name    : edubuntu-24.04.4-desktop-amd64.iso
File Length  : 6450806784 bytes (6151.97 MB)
Piece Length : 262144 bytes
Total Pieces : 24608
Info Hash    : f73430dbfaf0031f9c5fddcf0adc340456db4c91

Querying tracker https://torrent.ubuntu.com/announce for swarm peers...
Found 1 active peers in swarm:
  [01] 185.125.190.59:6918
```

---

## Testing Invariants

VortexTorrent includes comprehensive unit tests with synthetic mock peer servers:

* `bencode`: Asserts decoding of strings, integers (including leading zero / negative zero rejection), nested lists, and raw dictionary byte extraction.
* `peers`: Verifies Big-Endian port unpacking and malformed byte detection.
* `message`: Validates 68-byte handshake packet symmetry, 16KB block request formatting, and piece payload extraction.
* `bitfield`: Tests bitwise operations for piece availability indexing.
* `client`: Spawns a mock TCP server to test bidirectional handshakes and unchoke state transitions.
* `p2p`: Spawns an in-memory seeder delivering a 32KB test file across multiple pieces, validating full concurrent pipeline assembly and SHA-1 verification.

---

## License

MIT License. See [LICENSE](LICENSE) for details.
