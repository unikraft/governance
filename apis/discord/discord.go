package main

import (
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"os"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
	"gopkg.in/yaml.v3"
)

type Maintainers struct {
	Name    string `yaml:"name"  	default:"n/a"`
	Github  string `yaml:"github"  	default:"n/a"`
	Discord string `yaml:"discord" 	default:"n/a"`
}

type Reviewers struct {
	Name    string `yaml:"name" 	default:"n/a"`
	Github  string `yaml:"github" 	default:"n/a"`
	Discord string `yaml:"discord" 	default:"n/a"`
}

type BigFile struct {
	Name        string        `yaml:"name"`
	Description string        `yaml:"description"`
	Privacy     string        `yaml:"privacy"`
	Mnt         []Maintainers `yaml:"maintainers"`
	Rev         []Reviewers   `yaml:"reviewers"`
}

var (
	sig      BigFile
	BotToken = "MTA2OTAzNTc1NTYzNDE2Mzc5Mw.GpbSP3.gxFZjzvhsCUIcvvVoTCKfxg6wNu-TOYlk2DXFc"
	GuildID  = "711988332103598091"
)

func getConf(src string) {

	yamlFile, err := ioutil.ReadFile(src)
	if err != nil {
		fmt.Printf("Read error: %v", err)
	}

	err = yaml.Unmarshal(yamlFile, &sig)
	if err != nil {
		fmt.Printf("Unmarshal error: %v", err)
	}
}

func Start() {

	//starting new session instance
	discord, err := discordgo.New("Bot " + BotToken)
	if err != nil {
		log.Fatal(err)
	}

	//getting existing roles
	roles, err := getRoles(discord, GuildID)
	if err != nil {
		fmt.Println("Error getting roles: ", err)
		return
	}

	//getting existing channels
	channels, err := getChannels(discord, GuildID)
	if err != nil {
		fmt.Println("Error getting channels: ", err)
		return
	}

	//checking if we delete all roles and channels
	if os.Args[1] == "delete" {
		deleteAll(discord, roles, channels)
		return
	}

	//getting existing members
	members, err := getMembers(discord, GuildID)
	if err != nil {
		fmt.Println("Error getting members: ", err)
		return
	}

	//creating the maintainer role
	maint, err := createRole(discord, GuildID, "Maintainer", roles)
	if err != nil {
		fmt.Println("Error creating role de maintainer: ", err)
	}
	if maint != nil {
		fmt.Println("Role created: ", maint.Name)
		roles = append(roles, maint)
	}

	//creating the reviewer role
	rev, err := createRole(discord, GuildID, "Reviewer", roles)
	if err != nil {
		fmt.Println("Error creating role de reviewer: ", err)
	}
	if rev != nil {
		fmt.Println("Role created: ", rev.Name)
		roles = append(roles, rev)
	}

	//creating the contributor role
	cont, err := createRole(discord, GuildID, "Contributor", roles)
	if err != nil {
		fmt.Println("Error creating role de contributor: ", err)
	}
	if cont != nil {
		fmt.Println("Role created: ", cont.Name)
		roles = append(roles, cont)
	}

	chnName := strings.Split(sig.Name, "sig-")

	//creating the team role
	role, err := createRole(discord, GuildID, chnName[1], roles)
	if err != nil {
		fmt.Println("Error creating role de rol: ", err)
	}
	if role != nil {
		fmt.Println("Role created: ", role.Name)
		roles = append(roles, role)
	}

	ctbRole := findRole(roles, "Contributor")
	roleAssgn := findRole(roles, chnName[1])

	mntRole := findRole(roles, "Maintainer")
	for _, mnt := range sig.Mnt {
		mmnt := findUsr(members, mnt.Discord)

		err = assignRole(discord, GuildID, mmnt, mntRole.ID)
		if err != nil {
			fmt.Println("Error assigning role: ", err, " ", mntRole.Name)
		}
		err = assignRole(discord, GuildID, mmnt, ctbRole.ID)
		if err != nil {
			fmt.Println("Error assigning role: ", err, " ", ctbRole.Name)
		}
		err = assignRole(discord, GuildID, mmnt, roleAssgn.ID)
		if err != nil {
			fmt.Println("Error assigning role: ", err, " ", roleAssgn.Name)
		}
	}

	rvRole := findRole(roles, "Reviewer")
	for _, rv := range sig.Rev {
		rvMem := findUsr(members, rv.Discord)

		err = assignRole(discord, GuildID, rvMem, rvRole.ID)
		if err != nil {
			fmt.Println("Error assigning role: ", err, " ", rvRole.Name)
		}
		err = assignRole(discord, GuildID, rvMem, ctbRole.ID)
		if err != nil {
			fmt.Println("Error assigning role: ", err, " ", ctbRole.Name)
		}
		err = assignRole(discord, GuildID, rvMem, roleAssgn.ID)
		if err != nil {
			fmt.Println("Error assigning role: ", err, " ", roleAssgn.Name)
		}
	}

	chnCategory, err := createChn(discord, GuildID, chnName[1], discordgo.ChannelTypeGuildCategory, &channels)
	if err != nil {
		fmt.Println("Error creating channel category de rol: ", err)
	}

	chnText, err := createChn(discord, GuildID, chnName[1], discordgo.ChannelTypeGuildText, &channels)
	if err != nil {
		fmt.Println("Error creating channel text de rol: ", err)
	} else {
		fmt.Println("Channel created: ", chnText.Name)
	}

	chnVoice, err := createChn(discord, GuildID, chnName[1], discordgo.ChannelTypeGuildVoice, &channels)
	if err != nil {
		fmt.Println("Error creating channel voce de rol: ", err)
	} else {
		fmt.Println("Channel created: ", chnVoice.Name)
	}

	if chnText != nil {
		err = movChn(discord, GuildID, chnText.ID, chnCategory.ID)
		if err != nil {
			fmt.Println("Error moving channel: ", err, " ", chnText.Name)
		}
	}
	if chnVoice != nil {
		err = movChn(discord, GuildID, chnVoice.ID, chnCategory.ID)
		if err != nil {
			fmt.Println("Error moving channel: ", err, " ", chnVoice.Name)
		}
	}

	chnTeams, err := createChn(discord, GuildID, "Teams", discordgo.ChannelTypeGuildCategory, &channels)
	if err != nil {
		fmt.Println("Error creating channel category de teams: ", err)
	} else {

		chnMaintainer, err := createChn(discord, GuildID, "Maintainer", discordgo.ChannelTypeGuildText, &channels)
		if err != nil {
			fmt.Println("Error creating channel de maintainer: ", err)
		} else {
			fmt.Println("Channel created: ", chnMaintainer.Name)
		}

		chnReviewer, err := createChn(discord, GuildID, "Reviewer", discordgo.ChannelTypeGuildText, &channels)
		if err != nil {
			fmt.Println("Error creating channel de reviewer: ", err)
		} else {
			fmt.Println("Channel created: ", chnReviewer.Name)
		}

		chnContributor, err := createChn(discord, GuildID, "Contributor", discordgo.ChannelTypeGuildText, &channels)
		if err != nil {
			fmt.Println("Error creating channel de contributor: ", err)
		} else {
			fmt.Println("Channel created: ", chnContributor.Name)
		}

		if chnMaintainer != nil && chnTeams != nil {
			err = movChn(discord, GuildID, chnMaintainer.ID, chnTeams.ID)
			if err != nil {
				fmt.Println("Error moving channel: ", err)
			}
		}

		if chnReviewer != nil && chnTeams != nil {
			err = movChn(discord, GuildID, chnReviewer.ID, chnTeams.ID)
			if err != nil {
				fmt.Println("Error moving channel: ", err)
			}
		}

		if chnContributor != nil && chnTeams != nil {
			err = movChn(discord, GuildID, chnContributor.ID, chnTeams.ID)
			if err != nil {
				fmt.Println("Error moving channel: ", err)
			}
		}
	}

	discord.Open()
	defer discord.Close()

}

func chnPerms(discord *discordgo.Session, channel *discordgo.Channel, role *discordgo.Role) {

	err := discord.ChannelPermissionSet(channel.ID, role.ID, 0, discordgo.PermissionReadMessages|discordgo.PermissionSendMessages, 0)
	if err != nil {
		fmt.Println("Error setting channel permissions: ", err)
		return
	}

}

func deleteAll(discord *discordgo.Session, roles []*discordgo.Role, channels []*discordgo.Channel) {

	for _, role := range roles {
		if role.Name != "Unikraft Bot" && role.Name != "@everyone" && role.Name != "cobaibot" {
			err := discord.GuildRoleDelete(GuildID, role.ID)
			if err != nil {
				fmt.Println("Error deleting role: ", err)
				return
			}
		}
	}

	for _, channel := range channels {
		if channel.Name != "general" && channel.Name != "beta" {

			_, err := discord.ChannelDelete(channel.ID)
			if err != nil {
				fmt.Println("Error deleting channel: ", err)
				return
			}
			fmt.Println("Channel deleted: ", channel.Name)
		}
	}

}

func assignRole(s *discordgo.Session, guildID string, user *discordgo.Member, roleID string) error {

	if user == nil {
		return errors.New("User not found")
	}

	if len(user.Roles) == 250 {
		return errors.New("User already has 250 roles")
	}

	error := s.GuildMemberRoleAdd(guildID, user.User.ID, roleID)
	if error != nil {
		return error
	}
	return nil
}

func findUsr(members []*discordgo.Member, name string) *discordgo.Member {
	for _, member := range members {
		if member.User.Username == name {
			return member
		}
	}
	return nil
}

func findRole(roles []*discordgo.Role, name string) *discordgo.Role {
	for _, role := range roles {
		if role.Name == name {
			return role
		}
	}
	return nil
}

func checkAlreadyRole(roles []*discordgo.Role, name string) bool {
	for _, role := range roles {
		if role.Name == name {
			return true
		}
	}
	return false
}

func createRole(session *discordgo.Session, guildID string, name string, roles []*discordgo.Role) (*discordgo.Role, error) {

	if checkAlreadyRole(roles, name) {
		return nil, errors.New("Role already exists")
	}

	s1 := rand.NewSource(time.Now().UnixNano())
	r1 := rand.New(s1)
	clr := r1.Intn(16777216)

	role, err := session.GuildRoleCreate(guildID, &discordgo.RoleParams{
		Name:  name,
		Color: &clr,
	})
	if err != nil {
		return nil, err
	}

	return role, nil
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

func createChn(session *discordgo.Session, guild string, name string, typ discordgo.ChannelType, channels *[]*discordgo.Channel) (*discordgo.Channel, error) {

	for _, chn := range *channels {
		if chn.Name == name && chn.Type == typ {
			return nil, errors.New("Channel already exists")
		}
	}

	chn, err := session.GuildChannelCreate(guild, name, typ)
	if err != nil {
		return nil, err
	}

	return chn, nil
}

func getChannels(session *discordgo.Session, guildID string) ([]*discordgo.Channel, error) {
	channels, err := session.GuildChannels(guildID)
	if err != nil {
		return nil, err
	}
	return channels, nil
}

func getRoles(session *discordgo.Session, guildID string) ([]*discordgo.Role, error) {
	roles, err := session.GuildRoles(guildID)
	if err != nil {
		return nil, err
	}
	return roles, nil
}

func getMembers(session *discordgo.Session, guildID string) ([]*discordgo.Member, error) {
	members, err := session.GuildMembers(guildID, "", 1000)
	if err != nil {
		return nil, err
	}
	return members, nil
}

func messageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {

	if m.Author.ID == s.State.User.ID {
		return
	}

	switch {
	case strings.Contains(m.Content, "users"):
		{
			var Usrs string
			for _, v := range sig.Mnt {
				Usrs = Usrs + v.Discord + "\n"
			}

			for _, v := range sig.Rev {
				Usrs = Usrs + v.Discord + "\n"
			}

			s.ChannelMessageSend(m.ChannelID, Usrs)
		}
	case strings.Contains(m.Content, "ping"):
		s.ChannelMessageSend(m.ChannelID, "pong")

	}

}

func main() {

	fmt.Printf("Loading...\n")
	if os.Args[1] != "delete" {
		argsWithoutProg := os.Args[1]
		getConf(argsWithoutProg)
	}

	Start()

}
