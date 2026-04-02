import { useRef, useCallback, useEffect } from 'react';

// Flick-to-spin: drag an element to fling it, click to pause/unpause.
// While idle the CSS animation controls rotation. During interaction
// we disable CSS (flickActiveClass) and drive transform via JS,
// then hand back to CSS at the settled angle via animationDelay offset.

const DEAD_ZONE_SQ = 100;  // 10px radius — atan2 is unreliable closer to center
const DRAG_THRESHOLD = 3;  // cumulative degrees below which a gesture is a click
const STOP_VELOCITY = 5;   // deg/s — below this, deceleration is done
const FRICTION = 0.97;     // per-frame at 60fps

export default function useFlickSpin(flickActiveClass, animDuration, visible) {
  const elRef = useRef(null);
  const state = useRef({
    dragging: false,
    dragTotal: 0,
    paused: false,
    wasPaused: false,
    angle: 0,
    velocity: 0,
    rect: null,
    lastPointerAngle: 0,
    lastTime: 0,
    rafId: null,
  });

  const getPointerAngle = (clientX, clientY) => {
    const { rect } = state.current;
    return Math.atan2(
      clientY - (rect.top + rect.height / 2),
      clientX - (rect.left + rect.width / 2)
    );
  };

  const getCSSAngle = () => {
    const anim = elRef.current.getAnimations()[0];
    if (!anim) return 0;
    return (anim.effect.getComputedTiming().progress ?? 0) * 360;
  };

  const resumeCSS = useCallback((angle) => {
    const el = elRef.current;
    const s = state.current;
    s.paused = false;

    // restart CSS animation at the current angle by offsetting with negative delay
    const norm = ((angle % 360) + 360) % 360;
    el.style.transform = '';
    el.classList.remove(flickActiveClass);
    el.style.animation = 'none';
    void el.offsetHeight; // force reflow so removing + re-adding animation restarts it
    el.style.animation = '';
    el.style.animationDelay = `${-(norm / 360) * animDuration}s`;
  }, [flickActiveClass, animDuration]);

  const distSq = (e) => {
    const { rect } = state.current;
    const dx = e.clientX - (rect.left + rect.width / 2);
    const dy = e.clientY - (rect.top + rect.height / 2);
    return dx * dx + dy * dy;
  };

  const onPointerDown = useCallback((e) => {
    e.preventDefault();
    const el = elRef.current;
    if (!el) return;

    const s = state.current;

    // keep the current JS-tracked angle if paused or mid-deceleration, otherwise read from CSS
    s.angle = (s.paused || s.rafId) ? s.angle : getCSSAngle();

    if (s.rafId) {
      cancelAnimationFrame(s.rafId);
      s.rafId = null;
    }
    s.dragging = true;
    s.wasPaused = s.paused;
    s.dragTotal = 0;
    s.velocity = 0;
    s.rect = el.getBoundingClientRect();
    s.lastTime = performance.now();

    // only seed angle if outside the dead zone — atan2 is unreliable near center
    s.lastPointerAngle = (distSq(e) >= DEAD_ZONE_SQ)
      ? getPointerAngle(e.clientX, e.clientY)
      : null;

    el.classList.add(flickActiveClass);
    el.style.transform = `rotate(${s.angle}deg)`;
    el.setPointerCapture(e.pointerId);
  }, [flickActiveClass]);

  const onPointerMove = useCallback((e) => {
    const s = state.current;
    if (!s.dragging) return;

    const now = performance.now();
    const pointerAngle = getPointerAngle(e.clientX, e.clientY);

    // near center: atan2 is unstable, so null the seed and keep time synced
    if (distSq(e) < DEAD_ZONE_SQ) {
      s.lastPointerAngle = null;
      s.lastTime = now;
      return;
    }

    // first valid position after starting or passing through the dead zone — seed and skip
    if (s.lastPointerAngle === null) {
      s.lastPointerAngle = pointerAngle;
      s.lastTime = now;
      return;
    }

    let da = pointerAngle - s.lastPointerAngle;

    // unwrap angle delta to [-PI, PI] so crossing ±180° doesn't jump
    if (da > Math.PI) da -= 2 * Math.PI;
    if (da < -Math.PI) da += 2 * Math.PI;

    const daDeg = da * 180 / Math.PI;
    const dt = (now - s.lastTime) / 1000;

    s.angle += daDeg;
    s.dragTotal += Math.abs(daDeg);
    if (dt > 0) {
      s.velocity = s.velocity * 0.6 + (daDeg / dt) * 0.4; // EMA for smoother flicks
    }

    s.lastPointerAngle = pointerAngle;
    s.lastTime = now;
    elRef.current.style.transform = `rotate(${s.angle}deg)`;
  }, []);

  // browser aborted the gesture — restore pre-drag state
  const onPointerCancel = useCallback(() => {
    const s = state.current;
    if (!s.dragging) return;
    s.dragging = false;
    if (s.wasPaused) {
      s.paused = true;
    } else {
      resumeCSS(s.angle);
    }
  }, [resumeCSS]);

  const onPointerUp = useCallback(() => {
    const s = state.current;
    if (!s.dragging) return;
    s.dragging = false;

    if (s.dragTotal < DRAG_THRESHOLD) { // click, not drag
      s.paused = !s.paused;
      const el = elRef.current;
      if (s.paused) {
        el.classList.add(flickActiveClass);
        el.style.transform = `rotate(${s.angle}deg)`;
      } else {
        resumeCSS(s.angle);
      }
      return;
    }

    let velocity = s.velocity;
    let angle = s.angle;
    let lastTime = performance.now();

    const settle = (finalAngle) => {
      s.angle = finalAngle;
      if (s.wasPaused) {
        elRef.current.style.transform = `rotate(${finalAngle}deg)`;
      } else {
        resumeCSS(finalAngle);
      }
    };

    const decelerate = () => {
      if (!elRef.current) return;

      const now = performance.now();
      const dt = (now - lastTime) / 1000;
      lastTime = now;

      angle += velocity * dt;
      velocity *= Math.pow(FRICTION, dt * 60); // frame-rate-independent friction
      s.angle = angle;

      elRef.current.style.transform = `rotate(${angle}deg)`;

      if (Math.abs(velocity) > STOP_VELOCITY) {
        s.rafId = requestAnimationFrame(decelerate);
      } else {
        s.rafId = null;
        settle(angle);
      }
    };

    if (Math.abs(velocity) > STOP_VELOCITY) {
      s.rafId = requestAnimationFrame(decelerate);
    } else {
      settle(angle);
    }
  }, [flickActiveClass, resumeCSS]);

  // Reset when element is hidden without unmounting (e.g. compact mode toggle)
  useEffect(() => {
    if (!visible) {
      const s = state.current;
      if (s.rafId) cancelAnimationFrame(s.rafId);
      s.dragging = false;
      s.paused = false;
      s.angle = 0;
      s.velocity = 0;
      s.rafId = null;
    }
  }, [visible]);

  // Cleanup on unmount
  useEffect(() => {
    return () => {
      if (state.current.rafId) cancelAnimationFrame(state.current.rafId);
    };
  }, []);

  return { elRef, onPointerDown, onPointerMove, onPointerUp, onPointerCancel };
}
