# Connect a Discord bot

1. Create an application in the [Discord Developer Portal](https://discord.com/developers/applications).
2. Under **Installation**, enable **Guild Install** and select the \`bot\` and \`applications.commands\` scopes. Grant **View Channels**, **Manage Channels**, **Connect**, **Speak**, **Send Messages**, and **Attach Files** for the interview workflow. Administrator is unnecessary. See [Discord's bot setup guide](https://docs.discord.com/developers/quick-start/getting-started).
3. Open the generated installation link and add the bot to a server you manage. Create an **Interviews** category and a **Reviewer** role, then assign the role to the people who should join reviews.
4. Open **Connections & agents → Discord** in Teamrise. Paste the bot token into the local token field and choose **Connect bot**. Select the server, category, and Reviewer role, then choose **Verify & save Discord settings**. The bot token is entered again after restarting Teamrise.
5. Configure an AI provider and install the [voice runtime](VOICE-SETUP.md). Choose **Enable local voice** to register the interview commands for that server.

Teamrise creates a numbered voice room after the candidate accepts the interview notice. The assigned candidate, the bot, and the Reviewer role receive access. Server owners and Administrators retain access through Discord's permission rules. [Discord permissions](https://docs.discord.com/developers/topics/permissions)
