import { Check } from "lucide-react";
import AgentAvatar, { agentIdentity } from "./AgentAvatar";
import type { Agent, Candidate } from "./types";

export default function RobotTrail({
  candidate: c,
  agents,
  running,
  select,
}: {
  candidate: Candidate;
  agents: Agent[];
  running: boolean;
  select: (index: number) => void;
}) {
  const points = agents.map((_, i) => [38 + i * 86, i % 2 ? 78 : 30]);
  const waitingRoom = c.room.empty_since !== null || c.room.expired;
  return (
    <div className="robot-trail" aria-label={`Agent roadmap for ${c.name}`}>
      <svg
        className="trail-path"
        viewBox="0 0 592 120"
        preserveAspectRatio="none"
        aria-hidden="true"
      >
        {points.slice(0, -1).map(([x, y], i) => {
          const [nx, ny] = points[i + 1];
          return (
            <path
              key={i}
              d={`M${x} ${y} H${x + 43} V${ny} H${nx}`}
              stroke={
                c.runs[i]?.status === "done" ? agents[i].color : undefined
              }
              className={
                c.runs[i]?.status === "done" ? "link-done" : "link-idle"
              }
            />
          );
        })}
      </svg>
      {agents.map((agent, i) => {
        const run = c.runs[i];
        const status = run?.status ?? "idle";
        const active = status === "active";
        const blinking = active && running && !waitingRoom;
        return (
          <button
            key={agent.name}
            className={`robot-step ${status} ${blinking ? "blinking" : ""} ${active && !blinking ? "paused" : ""}`}
            style={{ "--agent-color": agent.color } as React.CSSProperties}
            onClick={() => select(i)}
            aria-label={`${agent.name}: ${active && !blinking ? "paused" : status}`}
            title={`${agentIdentity(agent.name).nickname} · ${agent.name}. ${agentIdentity(agent.name).personality}`}
          >
            <span className="robot-orb">
              <AgentAvatar agent={agent.name} />
              {status === "done" && (
                <span className="robot-check">
                  <Check size={9} />
                </span>
              )}
            </span>
            <span className="robot-label">
              {agent.name === "Fairness Auditor" ? "Auditor" : agent.name}
              <small>{agentIdentity(agent.name).nickname}</small>
            </span>
          </button>
        );
      })}
    </div>
  );
}
