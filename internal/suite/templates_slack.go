package suite

import (
	"can-i-compress-it/internal/domain"
)

func slackListChannels() domain.ToolDefinition {
	t := tool("list_channels", "List all channels in the Slack workspace.")
	t.Parameters = params(nil)
	return t
}

func slackPostMessage() domain.ToolDefinition {
	t := tool("post_message", "Post a new message to a Slack channel.")
	t.Parameters = params(map[string]any{
		"channel_id": obj("string", "ID of the channel to post in."),
		"text":       obj("string", "Message text to post."),
	})
	return t
}

func slackChannelHistory() domain.ToolDefinition {
	t := tool("get_channel_history", "Fetch recent messages from a Slack channel.")
	t.Parameters = params(map[string]any{
		"channel_id": obj("string", "ID of the channel to read."),
	})
	return t
}

func slackReplyThread() domain.ToolDefinition {
	t := tool("reply_to_thread", "Reply to an existing message thread in a Slack channel.")
	t.Parameters = params(map[string]any{
		"channel_id": obj("string", "ID of the channel containing the thread."),
		"thread_ts":  obj("string", "Timestamp of the parent message."),
		"text":       obj("string", "Reply text to post in the thread."),
	})
	return t
}

func slackAddReaction() domain.ToolDefinition {
	t := tool("add_reaction", "Add an emoji reaction to a Slack message.")
	t.Parameters = params(map[string]any{
		"channel_id": obj("string", "ID of the channel containing the message."),
		"timestamp":  obj("string", "Timestamp of the message to react to."),
		"reaction":   obj("string", "Name of the emoji reaction without colons."),
	})
	return t
}

func slackListUsers() domain.ToolDefinition {
	t := tool("list_users", "List users in the Slack workspace.")
	t.Parameters = params(map[string]any{
		"limit": obj("integer", "Number of users to retrieve."),
	})
	return t
}

func slackTasks() []domain.Task {
	listChannels := slackListChannels()
	postMessage := slackPostMessage()
	history := slackChannelHistory()
	replyThread := slackReplyThread()
	addReaction := slackAddReaction()
	listUsers := slackListUsers()
	return []domain.Task{
		{
			ID:       "sl_list_channels_001",
			Category: domain.CategoryImported,
			Prompt:   "Call list_channels exactly once with no arguments.",
			Tools:    []domain.ToolDefinition{listChannels, history},
			Expected: expected("list_channels", map[string]any{}),
		},
		{
			ID:       "sl_list_users_001",
			Category: domain.CategoryImported,
			Prompt:   "Call list_users exactly once. Arguments: limit: 20.",
			Tools:    []domain.ToolDefinition{listUsers, listChannels},
			Expected: expected("list_users", map[string]any{"limit": int64(20)}),
		},
		{
			ID:       "sl_post_message_001",
			Category: domain.CategoryImported,
			Prompt:   "Call post_message exactly once. Arguments: channel_id: C0123456789, text: Deploy finished successfully.",
			Tools:    []domain.ToolDefinition{postMessage, replyThread},
			Expected: expected("post_message", map[string]any{
				"channel_id": "C0123456789", "text": "Deploy finished successfully",
			}),
		},
		{
			ID:       "sl_history_001",
			Category: domain.CategoryImported,
			Prompt:   "Call get_channel_history exactly once. Arguments: channel_id: C0987654321.",
			Tools:    []domain.ToolDefinition{history, listChannels},
			Expected: expected("get_channel_history", map[string]any{
				"channel_id": "C0987654321",
			}),
		},
		{
			ID:       "sl_reply_thread_001",
			Category: domain.CategoryImported,
			Prompt:   "Call reply_to_thread exactly once. Arguments: channel_id: C0123456789, thread_ts: 1756000000.000100, text: On it now.",
			Tools:    []domain.ToolDefinition{replyThread, postMessage},
			Expected: expected("reply_to_thread", map[string]any{
				"channel_id": "C0123456789",
				"thread_ts":  "1756000000.000100",
				"text":       "On it now",
			}),
		},
		{
			ID:       "sl_add_reaction_001",
			Category: domain.CategoryImported,
			Prompt:   "Call add_reaction exactly once. Arguments: channel_id: C0424242424, timestamp: 1756000100.000200, reaction: white_check_mark.",
			Tools:    []domain.ToolDefinition{addReaction, postMessage, replyThread},
			Expected: expected("add_reaction", map[string]any{
				"channel_id": "C0424242424",
				"timestamp":  "1756000100.000200",
				"reaction":   "white_check_mark",
			}),
		},
		{
			ID:       "sl_chain_announce_001",
			Category: domain.CategoryMultiHop,
			Prompt:   "Step 1: call list_channels with no arguments. Step 2: call post_message. Arguments: channel_id: <from:list_channels.channel_id>, text: Weekly sync notes are up.",
			Tools:    []domain.ToolDefinition{listChannels, postMessage},
			Expected: expected("list_channels", map[string]any{}),
			MultiHop: []domain.ExpectedCall{
				expected("list_channels", map[string]any{}),
				expected("post_message", map[string]any{
					"channel_id": "<from:list_channels.channel_id>",
					"text":       "Weekly sync notes are up",
				}),
			},
		},
	}
}
