const identities: Record<string, { nickname: string; personality: string }> = {
  Scout: {
    nickname: "Atlas",
    personality: "The curious explorer. Always looking for a promising signal.",
  },
  Interviewer: {
    nickname: "Echo",
    personality: "The patient listener. Makes room for a thoughtful answer.",
  },
  Analyst: {
    nickname: "Prism",
    personality: "The methodical thinker. Finds the pattern in the evidence.",
  },
  "Fairness Auditor": {
    nickname: "Iris",
    personality: "The careful guardian. Takes a second look before judging.",
  },
  Ops: {
    nickname: "Bolt",
    personality: "The practical fixer. Keeps every connection running.",
  },
  Concierge: {
    nickname: "Pip",
    personality: "The warm host. Smooths out the unexpected.",
  },
  Chief: {
    nickname: "Nova",
    personality: "The calm coordinator. Sees how all the pieces fit.",
  },
};

export function agentIdentity(agent: string) {
  return (
    identities[agent] ?? {
      nickname: "Sol",
      personality: "A member of your agent team.",
    }
  );
}

/** Small vector characters stay sharp at every size and inherit workflow colors. */
export default function AgentAvatar({
  agent,
  size = 30,
}: {
  agent: string;
  size?: number;
}) {
  const common = {
    fill: "none",
    stroke: "currentColor",
    strokeWidth: 2,
    strokeLinecap: "round" as const,
    strokeLinejoin: "round" as const,
  };
  return (
    <svg
      className="agent-avatar"
      width={size}
      height={size}
      viewBox="0 0 48 48"
      aria-hidden="true"
      {...common}
    >
      {agent === "Scout" ? (
        <>
          <path d="M24 14V7l8-3" />
          <circle cx="33" cy="4" r="2" fill="currentColor" stroke="none" />
          <rect x="8" y="14" width="32" height="25" rx="12" />
          <path d="M4 23v8m40-8v8M18 40v4m12-4v4" />
          <circle cx="17" cy="25" r="5" />
          <circle cx="31" cy="25" r="5" />
          <path d="M22 25h4m-7 9q5 3 10-1" />
          <circle cx="18" cy="25" r="1.8" fill="currentColor" stroke="none" />
          <circle cx="32" cy="25" r="1.8" fill="currentColor" stroke="none" />
        </>
      ) : agent === "Interviewer" ? (
        <>
          <rect x="10" y="13" width="28" height="28" rx="11" />
          <path d="M6 27v-5a18 18 0 0 1 36 0v7M37 34h4q3 0 3-4" />
          <rect x="3" y="23" width="6" height="12" rx="3" />
          <rect x="39" y="23" width="6" height="10" rx="3" />
          <path d="M16 25q3-4 6 0m5 0q3-4 6 0m-15 8q6 6 12 0" />
          <circle cx="36" cy="34" r="2" fill="currentColor" stroke="none" />
        </>
      ) : agent === "Analyst" ? (
        <>
          <rect x="7" y="13" width="34" height="28" rx="5" />
          <path d="M24 13V6m-5 0h10M3 22v12m42-12v12M17 42v3m14-3v3" />
          <rect x="11" y="22" width="11" height="9" rx="2" />
          <rect x="26" y="22" width="11" height="9" rx="2" />
          <path d="M22 26h4m-6 10h8M14 18h3m4 0h3m4 0h6" />
          <path d="M16 25v3m15-3v3" />
        </>
      ) : agent === "Fairness Auditor" ? (
        <>
          <path d="m24 8 16 7v15q-2 10-16 15Q10 40 8 30V15zM24 8V3m-3 0h6" />
          <circle cx="17" cy="25" r="5" />
          <path d="m28 23 3 2 4-4M18 35q6-2 12 0M4 21v9m40-9v9" />
          <circle cx="17" cy="25" r="1.5" fill="currentColor" stroke="none" />
        </>
      ) : agent === "Ops" ? (
        <>
          <path d="m9 16 6-5h19l6 6v19l-5 5H14l-5-5zM22 11l5-8-1 6h5l-5 7" />
          <path d="M5 21H2v12h3m38-12h3v12h-3M16 37h16M16 19l5-1m7 0 5 2" />
          <rect
            x="14"
            y="24"
            width="7"
            height="6"
            rx="1"
            fill="currentColor"
            stroke="none"
          />
          <circle cx="31" cy="27" r="3.5" fill="currentColor" stroke="none" />
        </>
      ) : agent === "Concierge" ? (
        <>
          <path d="M10 16a14 12 0 0 1 28 0M7 17h34M24 4v3" />
          <circle cx="24" cy="3" r="2" fill="currentColor" stroke="none" />
          <path d="M9 18v12q1 10 15 10t15-10V18M4 24v8m40-8v8M15 26q3-4 6 0m6 0q3-4 6 0m-14 7q5 5 10 0M23 43l-6-3v7zm2 0 6-3v7z" />
        </>
      ) : agent === "Chief" ? (
        <>
          <path d="m13 14-2-9 9 6 4-8 5 8 8-6-2 9z" />
          <rect x="7" y="16" width="34" height="25" rx="9" />
          <path d="M3 23v10m42-10v10M14 23h20m-15 12h10M18 41v4m12-4v4" />
          <circle cx="17" cy="28" r="2.5" fill="currentColor" stroke="none" />
          <circle cx="31" cy="28" r="2.5" fill="currentColor" stroke="none" />
        </>
      ) : (
        <>
          <rect x="8" y="14" width="32" height="27" rx="9" />
          <path d="M24 14V7m-8 27h16" />
          <circle cx="17" cy="25" r="2" fill="currentColor" />
          <circle cx="31" cy="25" r="2" fill="currentColor" />
        </>
      )}
    </svg>
  );
}
