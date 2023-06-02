#!/bin/bash

sudo apt install golang-go -y 
go get github.com/bwmarrin/discordgo
go get gopkg.in/yaml.v3

if [ "$1" = "delete" ]; then
    go run discord.go delete
    exit 0
fi

pushd ../../teams/

for FILE in *.yaml; do
    go run ../apis/discord/discord.go $FILE; done

popd
