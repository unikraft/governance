package discord

import (
	"fmt"
	"os"
	"path"

	"github.com/bwmarrin/discordgo"
)

func translateChannelType(typ DiscordChannelType) discordgo.ChannelType {
	switch typ {
	case DiscordChannelTypeText:
		return discordgo.ChannelTypeGuildText
	case DiscordChannelTypeVoice:
		return discordgo.ChannelTypeGuildVoice
	}
}

func checkChannelAlreadyExists(channels []*discordgo.Channel, name string, typ discordgo.ChannelType) bool {
	for _, chn := range *channels {
		if chn.Name == name && chn.Type == typ {
			return true
		}
	}
	return false
}
