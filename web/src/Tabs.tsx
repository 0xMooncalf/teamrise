import { useLayoutEffect, useRef, useState, useId } from "react";

export default function Tabs({
  items,
  value,
  change,
  label,
  className = "",
}: {
  items: { id: string; label: string; count?: number }[];
  value: string;
  change: (id: string) => void;
  label: string;
  className?: string;
}) {
  const ref = useRef<HTMLDivElement>(null);
  const [ink, setInk] = useState({ left: 0, top: 0, width: 0, height: 0 });
  const id = useId();
  useLayoutEffect(() => {
    const node = ref.current;
    if (!node) return;
    const measure = () => {
      const tab = node.querySelector<HTMLButtonElement>(
        '[aria-selected="true"]',
      );
      if (tab)
        setInk({
          left: tab.offsetLeft,
          top: tab.offsetTop,
          width: tab.offsetWidth,
          height: tab.offsetHeight,
        });
    };
    measure();
    const observer = new ResizeObserver(measure);
    observer.observe(node);
    return () => observer.disconnect();
  }, [value, items.map((i) => i.label + String(i.count ?? "")).join("|")]);
  return (
    <div
      className={`segmented-tabs ${className}`}
      role="tablist"
      aria-label={label}
      ref={ref}
      onKeyDown={(e) => {
        const index = items.findIndex((i) => i.id === value);
        let next = index;
        if (e.key === "ArrowRight") next = (index + 1) % items.length;
        else if (e.key === "ArrowLeft")
          next = (index + items.length - 1) % items.length;
        else if (e.key === "Home") next = 0;
        else if (e.key === "End") next = items.length - 1;
        else return;
        e.preventDefault();
        change(items[next].id);
        ref.current
          ?.querySelectorAll<HTMLButtonElement>('[role="tab"]')
          [next]?.focus();
      }}
    >
      <span
        className="tab-ink"
        aria-hidden="true"
        style={{
          top: 0,
          transform: `translate(${ink.left}px,${ink.top}px)`,
          width: ink.width,
          height: ink.height,
        }}
      />
      {items.map((item) => (
        <button
          type="button"
          role="tab"
          id={`${id}-${item.id}`}
          aria-selected={value === item.id}
          tabIndex={value === item.id ? 0 : -1}
          key={item.id}
          onClick={() => change(item.id)}
        >
          {item.label}
          {item.count !== undefined && (
            <span className="tab-count">{item.count}</span>
          )}
        </button>
      ))}
    </div>
  );
}
