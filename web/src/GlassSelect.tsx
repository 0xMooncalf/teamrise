import { useId, useLayoutEffect, useRef, useState } from "react";
import { Check, ChevronDown } from "lucide-react";

export type SelectOption = { value: string; label: string };

export default function GlassSelect({
  label,
  value,
  options,
  onChange,
  disabled = false,
  placeholder = "Select an option",
}: {
  label: string;
  value: string;
  options: SelectOption[];
  onChange: (value: string) => void;
  disabled?: boolean;
  placeholder?: string;
}) {
  const id = useId();
  const trigger = useRef<HTMLButtonElement>(null);
  const menu = useRef<HTMLDivElement>(null);
  const [open, setOpen] = useState(false);
  const [active, setActive] = useState(0);
  const search = useRef({ text: "", at: 0 });
  const selected = options.findIndex((o) => o.value === value);
  const optionID = (i: number) => id + "-" + i;

  function position() {
    const button = trigger.current;
    const popup = menu.current;
    if (!button || !popup) return false;
    const anchor = button.getBoundingClientRect();
    const scrollArea = button.closest(".modal-scroll")?.getBoundingClientRect();
    const visual = window.visualViewport;
    const viewportHeight = visual
      ? visual.offsetTop + visual.height
      : window.innerHeight;
    const viewportWidth = visual
      ? visual.offsetLeft + visual.width
      : document.documentElement.clientWidth;
    const viewportTop = visual?.offsetTop ?? 0;
    const viewportLeft = visual?.offsetLeft ?? 0;
    const visibleTop = Math.max(viewportTop, scrollArea?.top ?? viewportTop);
    const visibleBottom = Math.min(
      viewportHeight,
      scrollArea?.bottom ?? viewportHeight,
    );
    if (anchor.bottom <= visibleTop || anchor.top >= visibleBottom)
      return false;
    const gap = 9;
    const down = viewportHeight - anchor.bottom - gap - 8;
    const up = anchor.top - viewportTop - gap - 8;
    const preferredHeight = Math.min(320, options.length * 47 + 10);
    const below = down >= preferredHeight || down >= up;
    const width = Math.min(anchor.width, viewportWidth - viewportLeft);
    popup.dataset.placement = below ? "below" : "above";
    Object.assign(popup.style, {
      width: width + "px",
      left:
        Math.max(viewportLeft, Math.min(anchor.left, viewportWidth - width)) +
        "px",
      top: below ? anchor.bottom + gap + "px" : "auto",
      bottom: below ? "auto" : window.innerHeight - anchor.top + gap + "px",
      maxHeight: Math.min(320, Math.max(0, below ? down : up)) + "px",
    });
    return true;
  }
  function show(i = Math.max(0, selected)) {
    if (!menu.current || disabled || !options.length) return;
    if (!position()) return;
    setActive(i);
    search.current = { text: "", at: 0 };
    const source = trigger.current;
    menu.current.showPopover(source ? { source } : undefined);
    setOpen(true);
    menu.current.focus({ preventScroll: true });
  }
  function hide(restore: boolean) {
    if (menu.current?.matches(":popover-open")) menu.current.hidePopover();
    setOpen(false);
    if (restore) trigger.current?.focus({ preventScroll: true });
  }
  function select(i: number) {
    const item = options[i];
    if (!item) return;
    hide(true);
    if (item.value !== value) onChange(item.value);
  }

  useLayoutEffect(() => {
    if (!open) return;
    function reposition(e: Event) {
      if (e.target instanceof Node && menu.current?.contains(e.target)) return;
      if (!position()) hide(true);
    }
    window.addEventListener("scroll", reposition, true);
    window.addEventListener("resize", reposition);
    window.visualViewport?.addEventListener("resize", reposition);
    window.visualViewport?.addEventListener("scroll", reposition);
    return () => {
      window.removeEventListener("scroll", reposition, true);
      window.removeEventListener("resize", reposition);
      window.visualViewport?.removeEventListener("resize", reposition);
      window.visualViewport?.removeEventListener("scroll", reposition);
    };
  }, [open, options.length]);

  useLayoutEffect(() => {
    if (!open) return;
    const row = menu.current?.children[active] as HTMLElement | undefined;
    const popup = menu.current;
    if (!row || !popup) return;
    if (row.offsetTop < popup.scrollTop) popup.scrollTop = row.offsetTop;
    else if (
      row.offsetTop + row.offsetHeight >
      popup.scrollTop + popup.clientHeight
    )
      popup.scrollTop = row.offsetTop + row.offsetHeight - popup.clientHeight;
  }, [open, active]);

  return (
    <span className="glass-select">
      <button
        ref={trigger}
        type="button"
        role="combobox"
        aria-haspopup="listbox"
        aria-label={label}
        aria-expanded={open}
        aria-controls={id}
        disabled={disabled || !options.length}
        className="glass-select-trigger"
        onClick={() => (open ? hide(true) : show())}
        onKeyDown={(e) => {
          if (!["ArrowDown", "ArrowUp", "Home", "End"].includes(e.key)) return;
          e.preventDefault();
          show(
            e.key === "Home"
              ? 0
              : e.key === "End"
                ? options.length - 1
                : Math.max(0, selected),
          );
        }}
      >
        <span>{options[selected]?.label ?? placeholder}</span>
        <ChevronDown size={15} aria-hidden="true" />
      </button>
      <div
        ref={menu}
        id={id}
        className="glass-select-menu"
        role="listbox"
        aria-label={label}
        tabIndex={-1}
        popover="auto"
        aria-activedescendant={open ? optionID(active) : undefined}
        onToggle={(e) => setOpen(e.currentTarget.matches(":popover-open"))}
        onKeyDown={(e) => {
          if (e.key === "Escape") {
            e.preventDefault();
            e.stopPropagation();
            hide(true);
            return;
          }
          if (e.key === "Tab") {
            hide(true);
            return;
          }
          if (e.key === "Enter" || e.key === " ") {
            e.preventDefault();
            select(active);
            return;
          }
          const last = options.length - 1;
          if (
            e.key === "ArrowDown" ||
            e.key === "ArrowUp" ||
            e.key === "Home" ||
            e.key === "End"
          ) {
            e.preventDefault();
            setActive(
              e.key === "Home"
                ? 0
                : e.key === "End"
                  ? last
                  : e.key === "ArrowDown"
                    ? Math.min(active + 1, last)
                    : Math.max(active - 1, 0),
            );
            return;
          }
          if (e.key.length === 1 && !e.ctrlKey && !e.metaKey && !e.altKey) {
            e.preventDefault();
            const now = Date.now();
            const text =
              (now - search.current.at < 650 ? search.current.text : "") +
              e.key.toLocaleLowerCase();
            search.current = { text, at: now };
            const next = options.findIndex((o) =>
              o.label.toLocaleLowerCase().startsWith(text),
            );
            if (next >= 0) setActive(next);
          }
        }}
      >
        {options.map((item, i) => (
          <div
            id={optionID(i)}
            key={item.value}
            role="option"
            aria-selected={item.value === value}
            className={i === active ? "active" : undefined}
            onPointerMove={() => setActive(i)}
            onClick={(e) => {
              e.preventDefault();
              select(i);
            }}
          >
            <span>{item.label}</span>
            {item.value === value && <Check size={14} aria-hidden="true" />}
          </div>
        ))}
      </div>
    </span>
  );
}
