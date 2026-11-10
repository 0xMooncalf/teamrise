# Run a Discord voice interview

Install **Node.js 22.12+** and **Python 3.12**. With Python on your PATH, run this from the Teamrise folder.

```powershell
powershell -ExecutionPolicy Bypass -File .\Install-Voice.ps1
```

The installer creates a local Python environment, installs locked Node and Python packages, and downloads the pinned English faster-whisper and Piper models. Pass \`-Python\` followed by the path to a Python 3.12 executable if needed. The machine running Teamrise needs UDP connectivity to Discord for voice.

1. Start Teamrise, [connect your bot](DISCORD-SETUP.md), and configure an AI provider.
2. Choose **Connections & agents → Discord → Enable local voice**. After changing skills, use **Reconnect voice & refresh skills** to update the available Discord choices.
3. In Discord, run \`/interview\`, pick a skill, accept the notice, and join the linked room from the same account. The bot introduces the skill and announces the question count and time limit. Edit the greeting and the default 75-minute limit in the Discord settings panel.
4. Answer spoken questions in English. For a written task, open **Open Chat** in the voice room and submit your answer with \`/interview-answer\`. The candidate sees the optional screenshot, while the AI uses the written assessment notes you supplied with the task.
5. Use \`/interview-repeat\` to repeat or retry, \`/interview-help\` to request a Reviewer, or \`/interview-stop\` to end the interview and retain the transcript for review. A human records the final decision.

If the bot is waiting for a spoken answer, five minutes of silence triggers voice recovery and a repeat of the current question. Written tasks keep waiting for a chat response. Separately, if the candidate leaves the room, they have five minutes to return before Teamrise removes the verified room. Answers and the interview deadline are retained during voice recovery.

If Discord connects but audio fails, check channel permissions and UDP connectivity, then choose **Reconnect voice & refresh skills**. If the AI request fails, verify the provider configuration and use \`/interview-repeat\` to retry from the saved state. See [voice dependency notices](VOICE-NOTICES.txt) for the runtime and model sources.
