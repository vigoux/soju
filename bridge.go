package soju

import (
	"strconv"
	"strings"
	"fmt"

	"gopkg.in/irc.v3"

	"git.sr.ht/~emersion/soju/ircutil"
)

type ircError struct {
	Message *irc.Message
}

func (err ircError) Error() string {
	return err.Message.String()
}

func newUnknownCommandError(cmd string) ircError {
	return ircError{&irc.Message{
		Command: irc.ERR_UNKNOWNCOMMAND,
		Params: []string{
			"*",
			cmd,
			"Unknown command",
		},
	}}
}

func newNeedMoreParamsError(cmd string) ircError {
	return ircError{&irc.Message{
		Command: irc.ERR_NEEDMOREPARAMS,
		Params: []string{
			"*",
			cmd,
			"Not enough parameters",
		},
	}}
}

func newChatHistoryError(subcommand string, target string) ircError {
	return ircError{&irc.Message{
		Command: "FAIL",
		Params:  []string{"CHATHISTORY", "MESSAGE_ERROR", subcommand, target, "Messages could not be retrieved"},
	}}
}

func parseMessageParams(msg *irc.Message, out ...*string) error {
	if len(msg.Params) < len(out) {
		return newNeedMoreParamsError(msg.Command)
	}
	for i := range out {
		if out[i] != nil {
			*out[i] = msg.Params[i]
		}
	}
	return nil
}

func forwardChannel(dc *downstreamConn, ch *upstreamChannel) {
	if !ch.complete {
		panic("Tried to forward a partial channel")
	}

	sendTopic(dc, ch)
	sendNames(dc, ch)
}

func sendTopic(dc *downstreamConn, ch *upstreamChannel) {
	downstreamName := dc.marshalEntity(ch.conn.network, ch.Name)

	if ch.Topic != "" {
		dc.SendMessage(&irc.Message{
			Prefix:  dc.srv.prefix(),
			Command: irc.RPL_TOPIC,
			Params:  []string{dc.nick, downstreamName, ch.Topic},
		})
		if ch.TopicWho != nil {
			topicWho := dc.marshalUserPrefix(ch.conn.network, ch.TopicWho)
			topicTime := strconv.FormatInt(ch.TopicTime.Unix(), 10)
			dc.SendMessage(&irc.Message{
				Prefix:  dc.srv.prefix(),
				Command: ircutil.RPL_TOPICWHOTIME,
				Params:  []string{dc.nick, downstreamName, topicWho.String(), topicTime},
			})
		}
	} else {
		dc.SendMessage(&irc.Message{
			Prefix:  dc.srv.prefix(),
			Command: irc.RPL_NOTOPIC,
			Params:  []string{dc.nick, downstreamName, "No topic is set"},
		})
	}
}

func sendNames(dc *downstreamConn, ch *upstreamChannel) {
	downstreamName := dc.marshalEntity(ch.conn.network, ch.Name)

	emptyNameReply := &irc.Message{
		Prefix:  dc.srv.prefix(),
		Command: irc.RPL_NAMREPLY,
		Params:  []string{dc.nick, string(ch.Status), downstreamName, ""},
	}
	maxLength := ircutil.MaxMessageLen - len(emptyNameReply.String())

	var buf strings.Builder
	for nick, memberships := range ch.Members {
		s := memberships.Format(dc.caps) + dc.marshalEntity(ch.conn.network, nick)

		n := buf.Len() + 1 + len(s)
		if buf.Len() != 0 && n > maxLength {
			// There's not enough space for the next space + nick.
			dc.SendMessage(&irc.Message{
				Prefix:  dc.srv.prefix(),
				Command: irc.RPL_NAMREPLY,
				Params:  []string{dc.nick, string(ch.Status), downstreamName, buf.String()},
			})
			buf.Reset()
		}

		if buf.Len() != 0 {
			buf.WriteByte(' ')
		}
		buf.WriteString(s)
	}

	if buf.Len() != 0 {
		dc.SendMessage(&irc.Message{
			Prefix:  dc.srv.prefix(),
			Command: irc.RPL_NAMREPLY,
			Params:  []string{dc.nick, string(ch.Status), downstreamName, buf.String()},
		})
	}

	dc.SendMessage(&irc.Message{
		Prefix:  dc.srv.prefix(),
		Command: irc.RPL_ENDOFNAMES,
		Params:  []string{dc.nick, downstreamName, "End of /NAMES list"},
	})
}

// applyChannelModes parses a mode string and mode arguments from a MODE message,
// and applies the corresponding channel mode and user membership changes on that channel.
//
// If ch.modes is nil, channel modes are not updated.
//
// needMarshaling is a list of indexes of mode arguments that represent entities
// that must be marshaled when sent downstream.
func applyChannelModes(ch *upstreamChannel, modeStr string, arguments []string) (needMarshaling map[int]struct{}, err error) {
	needMarshaling = make(map[int]struct{}, len(arguments))
	nextArgument := 0
	var plusMinus byte
outer:
	for i := 0; i < len(modeStr); i++ {
		mode := modeStr[i]
		if mode == '+' || mode == '-' {
			plusMinus = mode
			continue
		}
		if plusMinus != '+' && plusMinus != '-' {
			return nil, fmt.Errorf("malformed modestring %q: missing plus/minus", modeStr)
		}

		for _, membership := range ch.conn.availableMemberships {
			if membership.Mode == mode {
				if nextArgument >= len(arguments) {
					return nil, fmt.Errorf("malformed modestring %q: missing mode argument for %c%c", modeStr, plusMinus, mode)
				}
				member := arguments[nextArgument]
				if _, ok := ch.Members[member]; ok {
					if plusMinus == '+' {
						ch.Members[member].Add(ch.conn.availableMemberships, membership)
					} else {
						// TODO: for upstreams without multi-prefix, query the user modes again
						ch.Members[member].Remove(membership)
					}
				}
				needMarshaling[nextArgument] = struct{}{}
				nextArgument++
				continue outer
			}
		}

		mt, ok := ch.conn.availableChannelModes[mode]
		if !ok {
			continue
		}
		if mt == ircutil.ModeTypeB || (mt == ircutil.ModeTypeC && plusMinus == '+') {
			if plusMinus == '+' {
				var argument string
				// some sentitive arguments (such as channel keys) can be omitted for privacy
				// (this will only happen for RPL_CHANNELMODEIS, never for MODE messages)
				if nextArgument < len(arguments) {
					argument = arguments[nextArgument]
				}
				if ch.modes != nil {
					ch.modes[mode] = argument
				}
			} else {
				delete(ch.modes, mode)
			}
			nextArgument++
		} else if mt == ircutil.ModeTypeC || mt == ircutil.ModeTypeD {
			if plusMinus == '+' {
				if ch.modes != nil {
					ch.modes[mode] = ""
				}
			} else {
				delete(ch.modes, mode)
			}
		}
	}
	return needMarshaling, nil
}
