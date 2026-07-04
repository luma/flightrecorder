import { useEffect, useRef, type RefObject } from "react";

/**
 * Stack of currently-open overlays (popovers, drawers). Only the top-most
 * overlay responds to Escape / outside pointer-down, so stacked overlays
 * close one layer at a time.
 */
const overlayStack: symbol[] = [];

/**
 * Dismiss a floating element (menu, popover, drawer) on outside pointer-down
 * or Escape. `active` gates the listeners so they only exist while the
 * element is open. The latest `onDismiss` is kept in a ref, so passing an
 * inline callback does not re-subscribe the document listeners every render.
 */
export function useDismiss(
  ref: RefObject<HTMLElement | null>,
  onDismiss: () => void,
  active: boolean,
) {
  const onDismissRef = useRef(onDismiss);
  useEffect(() => {
    onDismissRef.current = onDismiss;
  });

  useEffect(() => {
    if (!active) return;
    const token = Symbol("overlay");
    overlayStack.push(token);
    const isTop = () => overlayStack[overlayStack.length - 1] === token;
    const onPointerDown = (event: PointerEvent) => {
      if (isTop() && ref.current && !ref.current.contains(event.target as Node)) {
        onDismissRef.current();
      }
    };
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape" && isTop()) {
        onDismissRef.current();
      }
    };
    document.addEventListener("pointerdown", onPointerDown);
    document.addEventListener("keydown", onKeyDown);
    return () => {
      const index = overlayStack.indexOf(token);
      if (index !== -1) overlayStack.splice(index, 1);
      document.removeEventListener("pointerdown", onPointerDown);
      document.removeEventListener("keydown", onKeyDown);
    };
  }, [active, ref]);
}
