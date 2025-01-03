# Discord Tunes

A simple streaming bot for discord. For individual use, please create your own discord app and supply your own token if you wish to run it. This prevents companies that start with Y from taking legal action as no service is being provided by me.

## Getting Started
1. Go to the [Discord Developer Portal](https://discord.com/developers/applications)
2. Create a "New Application" with the blue button
3. Give it a name and accept ToS
4. Set other customizations as you wish
5. On the "Bot" page, make sure you enable the following:
    - Server members intent
    - Message content intent
6. On the "Installation" page, set permissions as required. At a minimum give the bot voice and message related perms. For scopes give it "bot"
7. Copy the install link and install it into your server
8. Back on the "Bot" page, click "Reset Token", then "Yes do it"
9. Copy the generated token, and paste it into a file called "token.hide" within the root of this repo
10. Build the docker image locally with `docker build -t discord_tunes .`
11. Start up the container with `docker compose up -d`

> You can also adjust some settings within the `docker-compose.yml` file

You should now have the bot showing as online in your discord server, and it should be able to join calls / play audio.

## Features
- [x] Multi-server functionality
- [x] Able to join and leave voice calls in discord
- [ ] Youtube searching
- [x] Able to fetch audio stream from Youtube link
- [x] File downloads
- [x] Stream audio into voice calls
- [x] Song queues
- [x] Show song queue
- [x] Skip
