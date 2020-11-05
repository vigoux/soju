// Package ircutil exposes shared IRC helpers.
package ircutil

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"gopkg.in/irc.v3"
)

const (
	RPL_STATSPING     = "246"
	RPL_LOCALUSERS    = "265"
	RPL_GLOBALUSERS   = "266"
	RPL_CREATIONTIME  = "329"
	RPL_TOPICWHOTIME  = "333"
	ERR_INVALIDCAPCMD = "410"
)

const MaxMessageLen = 512

// The server-time layout, as defined in the IRCv3 spec.
const ServerTimeLayout = "2006-01-02T15:04:05.000Z"

func FormatTimeTag(t time.Time) irc.TagValue {
	return irc.TagValue(t.UTC().Format(ServerTimeLayout))
}

type UserModes string

func (ms UserModes) Has(c byte) bool {
	return strings.IndexByte(string(ms), c) >= 0
}

func (ms *UserModes) Add(c byte) {
	if !ms.Has(c) {
		*ms += UserModes(c)
	}
}

func (ms *UserModes) Del(c byte) {
	i := strings.IndexByte(string(*ms), c)
	if i >= 0 {
		*ms = (*ms)[:i] + (*ms)[i+1:]
	}
}

func (ms *UserModes) Apply(s string) error {
	var plusMinus byte
	for i := 0; i < len(s); i++ {
		switch c := s[i]; c {
		case '+', '-':
			plusMinus = c
		default:
			switch plusMinus {
			case '+':
				ms.Add(c)
			case '-':
				ms.Del(c)
			default:
				return fmt.Errorf("malformed modestring %q: missing plus/minus", s)
			}
		}
	}
	return nil
}

type ChannelModeType byte

// standard channel mode types, as explained in https://modern.ircdocs.horse/#mode-message
const (
	// modes that add or remove an address to or from a list
	ModeTypeA ChannelModeType = iota
	// modes that change a setting on a channel, and must always have a parameter
	ModeTypeB
	// modes that change a setting on a channel, and must have a parameter when being set, and no parameter when being unset
	ModeTypeC
	// modes that change a setting on a channel, and must not have a parameter
	ModeTypeD
)

var ChannelModeTypes = []ChannelModeType{
	ModeTypeA,
	ModeTypeB,
	ModeTypeC,
	ModeTypeD,
}

var StdChannelModes = map[byte]ChannelModeType{
	'b': ModeTypeA, // ban list
	'e': ModeTypeA, // ban exception list
	'I': ModeTypeA, // invite exception list
	'k': ModeTypeB, // channel key
	'l': ModeTypeC, // channel user limit
	'i': ModeTypeD, // channel is invite-only
	'm': ModeTypeD, // channel is moderated
	'n': ModeTypeD, // channel has no external messages
	's': ModeTypeD, // channel is secret
	't': ModeTypeD, // channel has protected topic
}

type ChannelModes map[byte]string

func (cm ChannelModes) Format() (modeString string, parameters []string) {
	var modesWithValues strings.Builder
	var modesWithoutValues strings.Builder
	parameters = make([]string, 0, 16)
	for mode, value := range cm {
		if value != "" {
			modesWithValues.WriteString(string(mode))
			parameters = append(parameters, value)
		} else {
			modesWithoutValues.WriteString(string(mode))
		}
	}
	modeString = "+" + modesWithValues.String() + modesWithoutValues.String()
	return
}

const StdChannelTypes = "#&+!"

type ChannelStatus byte

const (
	ChannelPublic  ChannelStatus = '='
	ChannelSecret  ChannelStatus = '@'
	ChannelPrivate ChannelStatus = '*'
)

func ParseChannelStatus(s string) (ChannelStatus, error) {
	if len(s) > 1 {
		return 0, fmt.Errorf("invalid channel status %q: more than one character", s)
	}
	switch cs := ChannelStatus(s[0]); cs {
	case ChannelPublic, ChannelSecret, ChannelPrivate:
		return cs, nil
	default:
		return 0, fmt.Errorf("invalid channel status %q: unknown status", s)
	}
}

type Membership struct {
	Mode   byte
	Prefix byte
}

var StdMemberships = []Membership{
	{'q', '~'}, // founder
	{'a', '&'}, // protected
	{'o', '@'}, // operator
	{'h', '%'}, // halfop
	{'v', '+'}, // voice
}

// memberships always sorted by descending membership rank
type Memberships []Membership

func (m *Memberships) Add(availableMemberships []Membership, newMembership Membership) {
	l := *m
	i := 0
	for _, availableMembership := range availableMemberships {
		if i >= len(l) {
			break
		}
		if l[i] == availableMembership {
			if availableMembership == newMembership {
				// we already have this membership
				return
			}
			i++
			continue
		}
		if availableMembership == newMembership {
			break
		}
	}
	// insert newMembership at i
	l = append(l, Membership{})
	copy(l[i+1:], l[i:])
	l[i] = newMembership
	*m = l
}

func (m *Memberships) Remove(oldMembership Membership) {
	l := *m
	for i, currentMembership := range l {
		if currentMembership == oldMembership {
			*m = append(l[:i], l[i+1:]...)
			return
		}
	}
}

func (m Memberships) Format(caps map[string]bool) string {
	if !caps["multi-prefix"] {
		if len(m) == 0 {
			return ""
		}
		return string(m[0].Prefix)
	}
	prefixes := make([]byte, len(m))
	for i, membership := range m {
		prefixes[i] = membership.Prefix
	}
	return string(prefixes)
}

func CopyClientTags(tags irc.Tags) irc.Tags {
	t := make(irc.Tags, len(tags))
	for k, v := range tags {
		if strings.HasPrefix(k, "+") {
			t[k] = v
		}
	}
	return t
}

func Join(channels, keys []string) []*irc.Message {
	// Put channels with a key first
	js := joinSorter{channels, keys}
	sort.Sort(&js)

	// Two spaces because there are three words (JOIN, channels and keys)
	maxLength := MaxMessageLen - (len("JOIN") + 2)

	var msgs []*irc.Message
	var channelsBuf, keysBuf strings.Builder
	for i, channel := range channels {
		key := keys[i]

		n := channelsBuf.Len() + keysBuf.Len() + 1 + len(channel)
		if key != "" {
			n += 1 + len(key)
		}

		if channelsBuf.Len() > 0 && n > maxLength {
			// No room for the new channel in this message
			params := []string{channelsBuf.String()}
			if keysBuf.Len() > 0 {
				params = append(params, keysBuf.String())
			}
			msgs = append(msgs, &irc.Message{Command: "JOIN", Params: params})
			channelsBuf.Reset()
			keysBuf.Reset()
		}

		if channelsBuf.Len() > 0 {
			channelsBuf.WriteByte(',')
		}
		channelsBuf.WriteString(channel)
		if key != "" {
			if keysBuf.Len() > 0 {
				keysBuf.WriteByte(',')
			}
			keysBuf.WriteString(key)
		}
	}
	if channelsBuf.Len() > 0 {
		params := []string{channelsBuf.String()}
		if keysBuf.Len() > 0 {
			params = append(params, keysBuf.String())
		}
		msgs = append(msgs, &irc.Message{Command: "JOIN", Params: params})
	}

	return msgs
}

type joinSorter struct {
	channels []string
	keys     []string
}

func (js *joinSorter) Len() int {
	return len(js.channels)
}

func (js *joinSorter) Less(i, j int) bool {
	if (js.keys[i] != "") != (js.keys[j] != "") {
		// Only one of the channels has a key
		return js.keys[i] != ""
	}
	return js.channels[i] < js.channels[j]
}

func (js *joinSorter) Swap(i, j int) {
	js.channels[i], js.channels[j] = js.channels[j], js.channels[i]
	js.keys[i], js.keys[j] = js.keys[j], js.keys[i]
}

// ParseCTCPMessage parses a CTCP message. CTCP is defined in
// https://tools.ietf.org/html/draft-oakley-irc-ctcp-02
func ParseCTCPMessage(msg *irc.Message) (cmd string, params string, ok bool) {
	if (msg.Command != "PRIVMSG" && msg.Command != "NOTICE") || len(msg.Params) < 2 {
		return "", "", false
	}
	text := msg.Params[1]

	if !strings.HasPrefix(text, "\x01") {
		return "", "", false
	}
	text = strings.Trim(text, "\x01")

	words := strings.SplitN(text, " ", 2)
	cmd = strings.ToUpper(words[0])
	if len(words) > 1 {
		params = words[1]
	}

	return cmd, params, true
}
