# 📖 Word of the Day Discord Bot

A discord bot written in **Go** using [discordgo](https://github.com/bwmarrin/discordgo).
It posts a random **Word of the Day** using:
  - [Random Word API](https://random-word-api.herokuapp.com/) → random word source
  - [Free Dictionary API](https://dictionaryapi.dev/) → definitions
The bot supports:
  - **Slash Command** `/wotd` (get a word + definition anytime) 
  - **Scheduled posting** (daily, at a time you choose)

## Setup
### 1. Clone and install 
```
git clone https://github.com/yourusername/discord-wotdbot
cd discord-wotdbot
go init wotd.go
go mod tidy
```
### 2. Create `.env` 
```
DISCORD_TOKEN=            # Discord bot token
GUILD_ID=                 # optional: restrict slash commands to one server (faster)
CHANNEL_ID=               # channel id of where it will post daily
TZ=America/New_York       # any valid IANA timezone
POST_AT=09:00             # 24h format HH:MM
```
### 3. Run the bot
```
go run wotd.go
```
or
```
go build wotd.go
./wotd
```
arigato 
