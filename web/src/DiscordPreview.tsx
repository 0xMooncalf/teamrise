import { useState } from "react";
import {
  DiscordMessage,
  DiscordMessages,
} from "@skyra/discord-components-react";
import { Hash, Link2 } from "lucide-react";
import type { State } from "./types";

const avatar = (name: string, color = "#648b80") =>
  `data:image/svg+xml,${encodeURIComponent(`<svg xmlns="http://www.w3.org/2000/svg" width="80" height="80"><rect width="80" height="80" rx="40" fill="${color}"/><text x="40" y="49" text-anchor="middle" fill="white" font-family="Arial" font-size="30">${name[0]}</text></svg>`)}`;

export default function DiscordPreview({ state }: { state?: State }) {
  const [visible, setVisible] = useState(16);
  const events = state?.events.slice(-60) ?? [];
  return (
    <section className="discord-panel">
      <div className="discord-heading">
        <Hash size={22} />
        <strong>pipeline-status</strong>
        <span>Hiring team updates</span>
        <span className="muted-pill">Preview · not connected to Discord</span>
      </div>
      <div className="discord-content">
        {events.length > visible && (
          <button
            className="button secondary"
            onClick={() => setVisible(visible + 16)}
          >
            Show earlier messages
          </button>
        )}
        <DiscordMessages>
          {events.length ? (
            events.slice(-visible).map((event) => (
              <DiscordMessage
                key={event.id}
                author={event.agent}
                avatar={avatar(
                  event.agent,
                  state?.agents.find((a) => a.name === event.agent)?.color,
                )}
                bot={event.agent !== "Human"}
                roleColor={
                  state?.agents.find((a) => a.name === event.agent)?.color
                }
                timestamp={new Date(event.time)}
              >
                <strong>
                  {event.candidate_name ? `${event.candidate_name} · ` : ""}
                </strong>
                {event.text}
              </DiscordMessage>
            ))
          ) : (
            <DiscordMessage
              author="Chief"
              avatar={avatar("Chief")}
              bot
              timestamp={new Date()}
            >
              Your hiring team is ready. Start the demo to see candidate updates
              here. This preview uses synthetic candidates.
            </DiscordMessage>
          )}
        </DiscordMessages>
      </div>
      <div className="discord-footnote">
        <Link2 size={14} />
        Preview messages only. This demo is not connected to a Discord server.
      </div>
    </section>
  );
}
