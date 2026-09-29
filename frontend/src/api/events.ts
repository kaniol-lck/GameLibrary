import { useEffect, useRef } from "react";
import { EventsOn } from "../../wailsjs/runtime/runtime";

/** Payload handler for a backend event. */
export type EventHandler = (payload: unknown) => void;

/**
 * Subscribes to backend events and unsubscribes on unmount.
 *
 * `EventsOn` returns an unsubscribe function that the previous code discarded, so
 * the handlers were never removed: React StrictMode's mount/unmount/remount cycle
 * registered each of them twice in development, and every Fast Refresh added
 * another five. Each backend event then triggered a full library reload as many
 * times as the handler had been registered.
 *
 * Handlers are read through a ref so that an inline arrow function does not
 * re-subscribe on every render; only a change to the set of event names does.
 */
export function useWailsEvents(handlers: Record<string, EventHandler>): void {
  const handlersRef = useRef(handlers);

  useEffect(() => {
    handlersRef.current = handlers;
  });

  // A stable key for the subscribed event names.
  const namesKey = Object.keys(handlers).sort().join("\u0000");

  useEffect(() => {
    const names = namesKey.split("\u0000").filter(Boolean);
    const unsubscribe = names.map((name) =>
      EventsOn(name, (payload: unknown) => {
        handlersRef.current[name]?.(payload);
      }),
    );
    return () => {
      for (const off of unsubscribe) {
        // A failing unsubscribe must not stop the others.
        try {
          off();
        } catch {
          /* the event bus is gone; nothing to do */
        }
      }
    };
  }, [namesKey]);
}
