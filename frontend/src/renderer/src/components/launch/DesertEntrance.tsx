import { useReducedMotion } from "motion/react";
import { useEffect, useLayoutEffect, useRef, useState } from "react";
import {
  BAYER8,
  WORDMARK_FONT,
  clamp01,
  drawText,
  easeInOut,
  hash,
  readPalette,
  threshold,
  toneFor,
} from "@/components/launch/DitherArt";
import { useEffectsEnabled } from "@/lib/appearance";
import { clientRuntime } from "@/lib/clientRuntime";
import { devPreview } from "@/lib/devPreview";

// Boot splash for the desktop window, in the brand's dithered grain: wind pours
// sand into three dune ranges, the sun (a moon by night) climbs behind them,
// and grains lift off the far crests into the wordmark. Once the backend has
// answered, a gust sweeps the desert away block by block, left to right, and
// the app shows through. Any key or click cuts straight to the gust.
//
// Perf: a grain is painted into an offscreen layer once it stops moving, so a
// frame blits that layer and draws only the grains still in flight.

const PITCH = 4;
const DOT = 3;
const ALPHA_STEPS = 12;
const BLOCK = 4;
const HOLD = 2.4;
const LATEST = 4.5;
const SWEEP = 0.7;
const SPREAD = 0.2;
const GUST = 0.55;

// `?entrance` plays the splash in a browser dev build; it otherwise belongs to
// the desktop app's main window only.
const plays =
  clientRuntime.windowKind === "main" &&
  (clientRuntime.kind === "electron" || devPreview("entrance") !== null);

// A grain travels a quadratic curve from (fx, fy) through (cx, cy) to its home
// (x, y), fading in over the first quarter of the trip.
type Grain = {
  x: number;
  y: number;
  fx: number;
  fy: number;
  cx: number;
  cy: number;
  start: number;
  settle: number;
  ease: (k: number) => number;
  tone: number;
  alpha: number;
  seed: number;
};

const random = (n: number) => hash(n) / 4294967296;
const linear = (k: number) => k;
const easeOut = (k: number) => 1 - (1 - k) ** 3;

const dune = (u: number) =>
  0.75 * (1 - Math.abs(Math.sin(u + 0.35 * Math.sin(u)))) +
  0.25 * (1 - Math.abs(Math.sin(2.7 * u + 1)));

const RANGES = [
  { base: 0.62, amp: 0.1, waves: 2.6, density: 0.5, delay: 0 },
  { base: 0.76, amp: 0.12, waves: 1.7, density: 0.62, delay: 0.2 },
  { base: 0.92, amp: 0.15, waves: 1.1, density: 0.85, delay: 0.4 },
];

// A disc lit from the upper left by night; by day, a sun cut by bands that
// thicken toward the horizon. Either way a faint halo around it.
function sunCoverage(dx: number, dy: number, d: number, dark: boolean): number {
  if (d > 1) return (dark ? 0.16 : 0.3) * (1 - (d - 1) / 0.5) ** 2;
  if (dark)
    return (
      0.15 +
      0.85 * clamp01(-0.55 * dx - 0.35 * dy + 0.75 * Math.sqrt(1 - d * d))
    );
  return dy > 0.05 && (dy * 6 + 0.3) % 1 < dy * 0.6 ? 0 : 1;
}

// The whole scene in grid cells. `horizon` is the far range's crest line: sky
// grains below it are hidden behind the dunes.
function buildDesert(cols: number, rows: number, dark: boolean) {
  const palette = readPalette();
  const tones = [
    palette.primary,
    palette.strong,
    palette.ink,
    ...palette.rainbow,
  ];
  const tone = (seed: number, ambient: boolean) =>
    tones.indexOf(toneFor(seed, ambient, palette));
  const grains: Grain[] = [];
  const add = (
    x: number,
    y: number,
    seed: number,
    g: Partial<Grain> & Pick<Grain, "start" | "settle" | "alpha">,
  ) =>
    grains.push({
      x,
      y,
      fx: x,
      fy: y,
      cx: x,
      cy: y,
      ease: linear,
      tone: tone(seed, false),
      seed,
      ...g,
    });

  const tops = RANGES.map((range, i) => {
    const phase = random(i + 7) * Math.PI;
    return Float32Array.from({ length: cols + 2 }, (_, c) => {
      const u = ((c - 1) / cols) * range.waves * Math.PI + phase;
      return rows * (range.base - range.amp * dune(u));
    });
  });
  const horizon = Float32Array.from({ length: cols }, (_, x) =>
    Math.ceil(tops[0][x + 1]),
  );

  // Front range first; each range further back fills only what is still open.
  const covered = new Float32Array(cols).fill(rows);
  for (let i = RANGES.length - 1; i >= 0; i--) {
    const range = RANGES[i];
    const top = tops[i];
    for (let x = 0; x < cols; x++) {
      const crest = Math.ceil(top[x + 1]);
      const facing = Math.max(-1, Math.min(1, 1.5 * (top[x + 2] - top[x])));
      for (let y = crest; y < covered[x]; y++) {
        const seed = hash(x + y * cols);
        // lit and shaded faces near the crest, thinning into haze lower down
        const depth = y - crest;
        const coverage =
          depth === 0
            ? 1
            : range.density * (0.55 + 0.45 * Math.exp(-depth / 14)) +
              0.35 * facing * Math.exp(-depth / 6);
        if (coverage < threshold(x, y, seed)) continue;
        const fall = 4 + 26 * random(seed + 1);
        const drift = 0.9 * fall;
        const start = range.delay + (0.6 * x) / cols + 0.3 * random(seed + 2);
        add(x, y, seed, {
          fx: x - drift,
          fy: y - fall,
          cx: x - drift / 2,
          cy: y - fall,
          start,
          settle: start + 0.55,
          alpha: (0.7 + 0.15 * i) * (0.85 + 0.15 * random(seed + 3)),
        });
      }
      covered[x] = Math.min(covered[x], crest);
    }
  }

  // The sun (or moon) rises from behind the far range.
  const radius = Math.min(rows * 0.16, cols * 0.14);
  const sunX = cols / 2;
  const sunY = rows * 0.54;
  const rise = rows * RANGES[0].base - (sunY - 1.5 * radius);
  for (let y = Math.floor(sunY - 1.5 * radius); y < sunY + 1.5 * radius; y++) {
    for (
      let x = Math.floor(sunX - 1.5 * radius);
      x < sunX + 1.5 * radius;
      x++
    ) {
      if (x < 0 || x >= cols || y < 0 || y >= horizon[x]) continue;
      const dx = (x - sunX) / radius;
      const dy = (y - sunY) / radius;
      const d = Math.hypot(dx, dy);
      if (d > 1.5) continue;
      const seed = hash(x + y * cols + 0x51ed);
      if (sunCoverage(dx, dy, d, dark) < threshold(x, y, seed)) continue;
      add(x, y, seed, {
        fy: y + rise,
        cy: y + rise / 2,
        ease: easeOut,
        start: 0.55,
        settle: 2,
        alpha: d <= 1 ? 0.9 : 0.6,
      });
    }
  }

  // Dust in the sky; stars at night.
  for (let x = 0; x < cols; x++) {
    for (let y = 0; y < horizon[x]; y++) {
      const seed = hash(x + y * cols + 0xd057);
      const noise = (seed % 4096) / 4096;
      if (noise >= 0.008) continue;
      const star = dark && noise < 0.0025;
      const start = 0.3 + 0.9 * random(seed + 1);
      add(x, y, seed, {
        tone: tone(seed, !star),
        start,
        settle: start + 1.6,
        alpha: star ? 0.55 + 0.35 * random(seed + 2) : 0.14,
      });
    }
  }

  // The wordmark, carried up off the far crests from upwind.
  const off = new OffscreenCanvas(cols, rows);
  const g = off.getContext("2d", { willReadFrequently: true })!;
  const wordTop = Math.round(rows * 0.07);
  const wordRows = Math.round(rows * 0.22);
  g.fillStyle = "#fff";
  g.translate(0, wordTop);
  drawText("jaz")(g, cols, wordRows);
  const letters = g.getImageData(0, wordTop, cols, wordRows).data;
  const word: [number, number, number][] = [];
  for (let y = 0; y < wordRows; y++) {
    for (let x = 0; x < cols; x++) {
      const seed = hash(x + (y + wordTop) * cols + 0x3a2);
      if (
        letters[(y * cols + x) * 4 + 3] / 255 >=
        threshold(x, y + wordTop, seed)
      )
        word.push([x, y + wordTop, seed]);
    }
  }
  const left = Math.min(...word.map(([x]) => x));
  const span = Math.max(1, Math.max(...word.map(([x]) => x)) - left);
  for (const [x, y, seed] of word) {
    const fx = x - 20 - 50 * random(seed + 1);
    const start = 0.9 + (0.55 * (x - left)) / span + 0.15 * random(seed + 2);
    add(x, y, seed, {
      fx,
      fy: horizon[Math.max(0, Math.min(cols - 1, Math.round(fx)))] - 1,
      cx: x - 6 - 10 * random(seed + 3),
      cy: y - 6 - 14 * random(seed + 4),
      ease: easeInOut,
      start,
      settle: start + 0.85,
      alpha: 0.85 + 0.15 * random(seed + 5),
    });
  }

  const streaks = Array.from({ length: Math.round(cols * 0.35) }, (_, i) => ({
    x: random(i * 5) * cols,
    y: Math.round(rows * (0.45 + 0.55 * random(i * 5 + 1))),
    speed: 20 + 50 * random(i * 5 + 2),
    length: 2 + Math.floor(4 * random(i * 5 + 3)),
    alpha: 0.15 + 0.25 * random(i * 5 + 4),
    tone: tone(hash(i), false),
  }));

  return {
    grains: grains.sort((a, b) => a.settle - b.settle),
    horizon,
    streaks,
    tones,
  };
}

// Collects dots by tone and quantized alpha so a frame costs one fill per bucket.
function painter(
  ctx: CanvasRenderingContext2D | OffscreenCanvasRenderingContext2D,
  tones: string[],
) {
  let paths: Path2D[] = [];
  return {
    dot(tone: number, alpha: number, x: number, y: number) {
      if (alpha < 0.03) return;
      const key =
        tone * ALPHA_STEPS + Math.round(Math.min(1, alpha) * (ALPHA_STEPS - 1));
      (paths[key] ??= new Path2D()).rect(
        Math.round(x) * PITCH,
        Math.round(y) * PITCH,
        DOT,
        DOT,
      );
    },
    flush() {
      paths.forEach((path, key) => {
        ctx.globalAlpha = (key % ALPHA_STEPS) / (ALPHA_STEPS - 1);
        ctx.fillStyle = tones[Math.floor(key / ALPHA_STEPS)];
        ctx.fill(path);
      });
      paths = [];
      ctx.globalAlpha = 1;
    },
  };
}

export function DesertEntrance({ ready }: { ready: boolean }) {
  const effects = useEffectsEnabled();
  const reduced = useReducedMotion();
  const [done, setDone] = useState(!plays);
  if (done || !effects || reduced) return null;
  return <Desert ready={ready} onDone={() => setDone(true)} />;
}

function Desert({ ready, onDone }: { ready: boolean; onDone: () => void }) {
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const readyRef = useRef(ready);
  const doneRef = useRef(onDone);
  useEffect(() => {
    readyRef.current = ready;
    doneRef.current = onDone;
  });

  // Layout effect: the ground is painted before the first frame, so the app
  // underneath never flashes through.
  useLayoutEffect(() => {
    const canvas = canvasRef.current!;
    const { clientWidth: width, clientHeight: height } = canvas;
    const dpr = window.devicePixelRatio || 1;
    canvas.width = Math.round(width * dpr);
    canvas.height = Math.round(height * dpr);
    const ctx = canvas.getContext("2d")!;
    ctx.scale(dpr, dpr);
    const ground = getComputedStyle(document.documentElement)
      .getPropertyValue("--color-bg")
      .trim();
    ctx.fillStyle = ground;
    ctx.fillRect(0, 0, width, height);

    let cancelled = false;
    let skipped = false;
    let raf = 0;
    const skip = () => {
      skipped = true;
    };
    window.addEventListener("pointerdown", skip, true);
    window.addEventListener("keydown", skip, true);

    const run = () => {
      if (cancelled) return;
      const cols = Math.ceil(width / PITCH);
      const rows = Math.ceil(height / PITCH);
      const { grains, horizon, streaks, tones } = buildDesert(
        cols,
        rows,
        document.documentElement.classList.contains("dark"),
      );

      const layer = new OffscreenCanvas(canvas.width, canvas.height);
      const layerCtx = layer.getContext("2d")!;
      layerCtx.scale(dpr, dpr);
      const baker = painter(layerCtx, tones);
      const paint = painter(ctx, tones);

      const blocksX = Math.ceil(cols / BLOCK);
      const blocks = Array.from(
        { length: blocksX * Math.ceil(rows / BLOCK) },
        (_, i) => {
          const [bx, by] = [i % blocksX, Math.floor(i / blocksX)];
          return {
            bx,
            by,
            at:
              (SWEEP * bx) / blocksX +
              (SPREAD * BAYER8[(by % 8) * 8 + (bx % 8)]) / 64,
          };
        },
      );
      const clearAt = Float32Array.from(blocks, (b) => b.at);
      blocks.sort((a, b) => a.at - b.at);
      const cleared = new Path2D();
      const gusts = grains.filter((g) => g.seed % 3 === 0);
      const side = BLOCK * PITCH;

      let baked = 0;
      let swept = 0;
      let revealAt: number | null = null;
      const t0 = performance.now();
      const frame = (now: number) => {
        const t = (now - t0) / 1000;
        if (
          revealAt === null &&
          (skipped || t >= LATEST || (t >= HOLD && readyRef.current))
        ) {
          revealAt = t;
          canvas.style.pointerEvents = "none";
        }
        const r = revealAt === null ? -1 : t - revealAt;

        for (; baked < grains.length && grains[baked].settle <= t; baked++) {
          const g = grains[baked];
          baker.dot(g.tone, g.alpha, g.x, g.y);
        }
        baker.flush();

        ctx.globalCompositeOperation = "source-over";
        ctx.fillStyle = ground;
        ctx.fillRect(0, 0, width, height);
        ctx.drawImage(layer, 0, 0, width, height);
        for (let i = baked; i < grains.length; i++) {
          const g = grains[i];
          if (g.start > t) continue;
          const raw = (t - g.start) / (g.settle - g.start);
          const k = g.ease(raw);
          const u = 1 - k;
          const x = u * u * g.fx + 2 * u * k * g.cx + k * k * g.x;
          const y = u * u * g.fy + 2 * u * k * g.cy + k * k * g.y;
          // sky grains still below the far crests are behind the dunes
          if (g.y < horizon[g.x] && y >= horizon[Math.round(x)]) continue;
          paint.dot(g.tone, g.alpha * Math.min(1, raw * 4), x, y);
        }
        const calm = clamp01((t - 0.2) / 0.6) * (1 - clamp01(r / GUST));
        for (const s of streaks) {
          const head = ((s.x + s.speed * t) % (cols + 12)) - 6;
          for (let j = 0; j < s.length; j++)
            paint.dot(
              s.tone,
              s.alpha * calm * (1 - j / s.length),
              head - j,
              s.y,
            );
        }
        paint.flush();

        if (r >= 0) {
          for (; swept < blocks.length && blocks[swept].at <= r; swept++) {
            cleared.rect(
              blocks[swept].bx * side,
              blocks[swept].by * side,
              side,
              side,
            );
          }
          ctx.globalCompositeOperation = "destination-out";
          ctx.fill(cleared);
          ctx.globalCompositeOperation = "source-over";
          // grains in a cleared block blow downwind over the app as it appears
          for (const g of gusts) {
            const k =
              (r -
                clearAt[
                  Math.floor(g.y / BLOCK) * blocksX + Math.floor(g.x / BLOCK)
                ]) /
              GUST;
            if (k < 0 || k >= 1) continue;
            paint.dot(
              g.tone,
              g.alpha * (1 - k),
              g.x + 46 * k * k,
              g.y - 10 * k * random(g.seed),
            );
          }
          paint.flush();
          if (r > SWEEP + SPREAD + GUST) {
            doneRef.current();
            return;
          }
        }
        raf = requestAnimationFrame(frame);
      };
      raf = requestAnimationFrame(frame);
    };

    // The wordmark is sampled from Inter, so load it before building.
    void document.fonts.load(WORDMARK_FONT).then(run, run);
    return () => {
      cancelled = true;
      cancelAnimationFrame(raf);
      window.removeEventListener("pointerdown", skip, true);
      window.removeEventListener("keydown", skip, true);
    };
  }, []);

  return (
    <canvas
      ref={canvasRef}
      aria-hidden
      className="fixed inset-0 z-[120] size-full"
    />
  );
}
