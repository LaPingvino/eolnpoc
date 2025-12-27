Ephemeral Open Location Network Proof of Concept
================================================

Open Location Network is a flexible idea that this projects intends to bootstrap by giving an example of and proving the basic concepts of the idea. The Ephemeral in the name means that it doesn't try to be persistent in any way, so you shouldn't in any way use it to save any information. This project is for development of concepts, not for storage and I will actively try to avoid storing anything.

## Quick Start: Using OLN with NATS

This PoC uses NATS.io for P2P message distribution. No complex setup needed!

### Build the OLN Node

```bash
go build -o olnnode ./cmd/olnnode/main.go
```

### Publish a Message

```bash
./olnnode publish "Hello #OLN world! #test"
```

### Listen for Messages

```bash
./olnnode listen
```

In another terminal, publish messages and they'll appear in the listener!

### Connect to a Different NATS Server

```bash
./olnnode server nats://localhost:4222 listen
```

The default server is `nats://demo.nats.io:4222`, which is public and requires no setup.

---

So what is Open Location Network? Short OLN (and you can also replace the L with Listening) its goal is to create an open source P2P internet backchannel ad-hoc protocol of sorts that grows with our concept of what the internet is. The idea is that you can pass new messages through to other participants in the network and any participant can index them any way it wants and enable people to search for what they need.

What would you index on? Anything that is or will be a standardized way to refer to things. E.g. hashtags for things we talk about, URLs to relate messages to parts of the internet, timestamps and of course the Location part, plustags.

Plustags is my name for the idea of using Google's pluscodes as some kind of hashtags for location. My idea with OLN is to make it possible to find messages that are relevant for you based on subject and location, and be completely oblivious of the rest of things that go on. And of course the participants in the network should be able to be fully opinionated about what they index etc, for example things that relate to specific subjects and/or regions.

Anyone is free to help. How to help? Just create anything that would make any of these ideas closer to reality, or develop upon these ideas to make something cool yourself. What would you do if the internet had a backchannel that is open to whispering anything to anyone willing to listen? That's why OLN can just as well mean Open Listening Network.

Following the presentation that I put on https://hackmd.io/@lapingvino/SJl7Ap8UH

# Implementing OLN

Creating a JSON standard for message passing.

---

## What is OLN?

OLN is short for Open Location Network (because it aims to enable Twitter-style P2P communication but relevant for your location) or Open Listening Network (because it aims to be a backchannel of the internet). Also it's a reference to OLC which I use to implement location support.

---

It is whatever we all make of it, but this document aims to provide a starting point we can all agree on.

---

It consists of a JSON format that servers and peers can communicate with to each other and a parseable message format very similar to Twitter messaging.

---

My vision is that you can use an OLN access point (e.g. a website, a command line client, a mobile phone app, a PWA) to search for messages relevant to you by place and subject and send your own messages be it by providing them to a push-enabled peer or making them available on your own peer, or both.

---

Testing should show what works and what not, and how we can improve this network. The different elements of the JSON should provide enough options to extend the format over time and according to the needs you apply this to.

---

## JSON format requirements (all [except Raw text] optional?):
- Raw text
- Origin
    - Username
    - Server or P2P
    - Public key bitcoin-address style

---

- Timestamp for time of publication
- TTL (time in days for relevance since timestamp)
- Hops (measure relevance for unrequested propagation)
- Signing
- Encryption

---

- Hash (for message identification / content based addressing)
    - Based on just raw text? Other fields derived from raw text?
- Tags (hashtag, plustag)

---

- Probably different types?
    - Messages
    - Search index
        - Search on origin, tags, hash
    - Other known feeds
    - Server information
        - Fixed tag for non-changing feeds, otherwise answer query

---

## Example format

```json
{
    server: {
       link: "", // protocol:link or /ipfs/-style
       name: "",
       pubkey: "",
       acceptpush: false // true if it accepts P2P pushing of new messages and index information
    }
    messages: {
        "hash": {
            raw: "", // can be encrypted, receiver can try decryption with either private key or AES symmetrical encryption
            origin: {
                display: "",
                pubkey: "",
                servername: ""
            }
            sig: "",
            timestamp: "",
            ttl: 0,
            hops: 0,
            tags: ["", "", "", ...]
        },
        "anotherhash": {
                ...
        }
    },
    index: {
        "tag": ["hash", "hash", "link", ...],
        "anothertag": ["hash", "hash", "link" ...],
        "paddedplustag": ["fullplustag", "hash", "link", ...],
        "link": ["hash", "hash", ...],
        ...
    },
    feeds: ["link", "link", ...],
    push: ["link", "link", ...]
}
```

---

## Endpoints

HTTP(S)-endpoints, usually .json, can be any location on server
If server is enabled to do so, answer requests
Other kinds of endpoints, like smtp, irc, ipfs-pubsub, ssh, ...

---

## Message format notes

Mostly Twitter-style:
- @mentioning for mentioning people by origin or messages by hash
- #hashtags for topics
- http:// and https:// for links, embed when possible/feeling like it (e.g. images) and when safe
- /ipfs/ and /ipns/ links that should be treated similar to http(s)

---

- plustags (locations mentioned by full form pluscode, padded with 00 for increasing range)
    - *Press down arrow to read more!*
- key: entries for any other kind of information
- \[Alttext](mention,tag,link,key) for providing alternative representation, e.g. providing a human-readable location for plustags

----

## Explaining plustags

Plustags use Google's pluscodes, also known as OLC, for tagging a message with location information. These codes are short and variable precision, and indicate a block instead of a point. Check https://plus.codes to experiment yourself.

----

The codes use base20 pairs to indicate blocks on a grid. Every next two characters of the code go a level deeper. A code for a normal location looks like 6FG22222+22. To remove precision for personal safety, you can remove data:
- 6FG22222+ is a level up
- 6FG22200+ is again a level up, padded with 00 (not part of the base20 charset)
- 6FG20000+ is another level up, etc

----

When adding plustags to the index, it's nice to also add the plustags to the plustag one level higher in the index, and this one to one level higher until reaching 00000000+. This takes very little space but enables searching for locations by simple string comparison and traversal.

---

I'm a bit undecided about a format for time/date in messages, ISO timestamps might be too limited for representing inexact time.

---

## Implementation

Client can implement local indexing of feed information under all of mentioned elements. Need to unpack Alttext-representation. Plustags can add simple or more complex proximity search algorithm, e.g. string-based on several layers.
Need to have pubkey and hash format, I think it's good to use [ipfs multihash format](https://github.com/multiformats/multihash) in base58 like ipfs does, with sha2-256 prefered/default for hash and ed25519-pub for pubkey?

---

## Sweet, where to connect?

I would like to be able to connect to each other asap over OLN itself, however we implement it, but until then, #oln on Freenode (IRC) is the place to go. Update: I also just created [r/oln](https://reddit.com/r/oln) to talk about OLN stuff. For as long as OLN isn't a thing yet...
