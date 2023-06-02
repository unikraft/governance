package discord

import (
	"fmt"
	"os"
	"path"

	"github.com/bwmarrin/discordgo"
)

type DiscordCategory struct {
	Name     string           `yaml:"name"`
	Private  bool             `yaml:"private"`
	Archived bool             `yaml:"archived"`
	Roles    []string         `yaml:"roles"`
	Members  []string         `yaml:"members"`
	Channels []DiscordChannel `yaml:"channels"`
}

type DiscordChannelType string

const (
	DiscordChannelTypeText  DiscordChannelType = "text"
	DiscordChannelTypeVoice DiscordChannelType = "voice"
)

var (
	DiscordChannelTypes = [...]DiscordChannelType{
		DiscordChannelTypeText,
		DiscordChannelTypeVoice,
	}
)

type DiscordChannel struct {
	Name        string               `yaml:"name"`
	Emoji       string               `yaml:"emoji"`
	Description string               `yaml:"description"`
	Private     bool                 `yaml:"private"`
	Archived    bool                 `yaml:"archived"`
	Type        []DiscordChannelType `yaml:"type"`
	Roles       []string             `yaml:"roles"`
	Members     []string             `yaml:"members"`
}

func NewChannel(session *discordgo.Session, guild string, name string, typ string, channels *[]*discordgo.Channel) (*discordgo.Channel, error) {

	channelType := translateChannelType(typ)

	if checkChannelAlreadyExists(*channels, name, channelType) {
		return nil, errors.New("channel already exists")
	}

	chn, err := session.GuildChannelCreate(guild, name, channelType)
	if err != nil {
		return nil, err
	}

	return chn, nil
}

func movChn(session *discordgo.Session, guildID string, channelID string, parentID string) error {
	_, err := session.ChannelEditComplex(channelID, &discordgo.ChannelEdit{
		ParentID: parentID,
	})
	if err != nil {
		return err
	}
	return nil
}
