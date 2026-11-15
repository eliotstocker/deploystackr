<script lang="ts">
  import { onMount, onDestroy, type Snippet } from "svelte";

  /**
   * Component Props
   */
  interface Props {
    children?: Snippet;
    dark?: boolean;
    theme?: string;
    speed?: number;
    diffusion?: number; // "High Diffusion" default
    opacity?: number | null; // null defaults to 0.25 (light) and 0.15 (dark)
    interactive?: boolean;
    showNoise?: boolean;
    className?: string;
  }

  let {
    children,
    dark = false,
    theme,
    speed = 0.25,
    diffusion = 1.75,
    opacity = null,
    interactive = true,
    showNoise = true,
    className = "",
  }: Props = $props();

  // Curated color themes: calibrated translucent RGB values for harmonious blending
  export interface ColorTheme {
    name: string;
    light: number[][];
    dark: number[][];
  }

  export const THEMES: ColorTheme[] = [
    {
      name: "Cobalt & Amber",
      // Complementary pair: Azure/Cobalt Blue ↔ Golden Amber
      light: [
        [59, 130, 246], // Vibrant Azure
        [245, 158, 11], // Warm Amber
        [37, 99, 235], // Deep Cobalt
        [251, 191, 36], // Radiant Gold
        [96, 165, 250], // Soft Azure
      ],
      dark: [
        [37, 99, 235], // Midnight Cobalt
        [245, 158, 11], // Molten Amber
        [29, 78, 216], // Royal Sapphire
        [217, 119, 6], // Deep Gold
        [59, 130, 246], // Electric Azure
      ],
    },
    {
      name: "Teal & Coral",
      // Complementary pair: Aqua Teal/Cyan ↔ Tangerine Coral
      light: [
        [13, 148, 136], // Deep Teal
        [249, 115, 22], // Radiant Coral
        [20, 184, 166], // Seafoam Aqua
        [251, 146, 60], // Warm Tangerine
        [6, 182, 212], // Cyan Highlight
      ],
      dark: [
        [15, 118, 110], // Abyssal Teal
        [234, 88, 12], // Glowing Coral
        [20, 184, 166], // Neon Seafoam
        [249, 115, 22], // Ember Tangerine
        [13, 148, 136], // Deep Cyan
      ],
    },
    {
      name: "Violet & Citron",
      // Complementary pair: Royal Violet ↔ Sunny Citron Yellow
      light: [
        [147, 51, 234], // Electric Violet
        [234, 179, 8], // Sunny Citron
        [168, 85, 247], // Orchid Lavender
        [250, 204, 21], // Bright Lemon Citron
        [126, 34, 206], // Deep Violet
      ],
      dark: [
        [126, 34, 206], // Ultraviolet
        [202, 138, 4], // Golden Citron
        [147, 51, 234], // Vivid Violet
        [234, 179, 8], // Radiant Citron
        [109, 40, 217], // Midnight Orchid
      ],
    },
    {
      name: "Emerald & Rose",
      // Complementary pair: Lush Emerald Green ↔ Petal Rose / Raspberry
      light: [
        [16, 185, 129], // Mint Emerald
        [244, 63, 94], // Rose Ruby
        [5, 150, 105], // Deep Mint
        [251, 113, 133], // Soft Rose
        [52, 211, 153], // Seafoam Green
      ],
      dark: [
        [5, 150, 105], // Forest Emerald
        [225, 29, 72], // Neon Rose
        [4, 120, 87], // Deep Emerald
        [244, 63, 94], // Vivid Rose Ruby
        [16, 185, 129], // Mint Glow
      ],
    },
    {
      name: "Indigo & Peach",
      // Complementary pair: Twilight Indigo ↔ Sunset Peach / Honey
      light: [
        [79, 70, 229], // Twilight Indigo
        [251, 146, 60], // Warm Sunset Peach
        [99, 102, 241], // Soft Indigo
        [253, 186, 116], // Luminous Peach
        [67, 56, 202], // Royal Indigo
      ],
      dark: [
        [67, 56, 202], // Midnight Indigo
        [245, 158, 11], // Ember Peach
        [79, 70, 229], // Electric Indigo
        [234, 88, 12], // Sunset Copper
        [99, 102, 241], // Royal Indigo
      ],
    },
    {
      name: "Plum & Chartreuse",
      // Complementary pair: Velvet Plum / Orchid ↔ Spring Chartreuse / Lime
      light: [
        [192, 38, 211], // Velvet Plum
        [132, 204, 22], // Spring Chartreuse
        [162, 28, 175], // Deep Orchid
        [163, 230, 53], // Crisp Lime Chartreuse
        [217, 70, 239], // Magenta Orchid
      ],
      dark: [
        [147, 51, 234], // Deep Plum
        [101, 163, 13], // Electric Chartreuse
        [192, 38, 211], // Neon Orchid
        [132, 204, 22], // Bright Lime
        [126, 34, 206], // Velvet Violet
      ],
    },
    {
      name: "Cerulean & Flame",
      // Complementary pair: Sky Cerulean Blue ↔ Vermilion Flame Red-Orange
      light: [
        [14, 165, 233], // Sky Cerulean
        [239, 68, 68], // Vermilion Flame
        [56, 189, 248], // Ice Azure
        [248, 113, 113], // Soft Flame
        [2, 132, 199], // Deep Cerulean
      ],
      dark: [
        [2, 132, 199], // Ocean Cerulean
        [220, 38, 38], // Molten Flame
        [14, 165, 233], // Electric Azure
        [239, 68, 68], // Neon Vermilion
        [3, 105, 161] // Abyssal Blue
      ],
    },
  ];

  /**
   * Helper to pick a random theme, optionally excluding the current one to ensure a visible change
   */
  function getRandomTheme(excludeName?: string): ColorTheme {
    const pool = excludeName
      ? THEMES.filter((t) => t.name.toLowerCase() !== excludeName.toLowerCase())
      : THEMES;
    const candidates = pool.length > 0 ? pool : THEMES;
    return candidates[Math.floor(Math.random() * candidates.length)];
  }

  // Pick a random theme on application load
  let selectedTheme = $state(getRandomTheme());
  let prevDark: boolean | undefined = undefined;

  // Switch to a new random theme whenever toggling between light and dark mode
  $effect(() => {
    const isDark = dark;
    if (prevDark !== undefined && isDark !== prevDark) {
      selectedTheme = getRandomTheme(selectedTheme.name);
    }
    prevDark = isDark;
  });

  // Resolve active theme: respect explicit theme prop if provided, else use random selected theme
  const currentTheme = $derived(
    theme
      ? (THEMES.find((t) => t.name.toLowerCase() === theme.toLowerCase()) ??
          selectedTheme)
      : selectedTheme,
  );

  /**
   * Calculates a perceptual intensity multiplier based on ITU-R BT.709 relative luminance.
   * Equalizes visual contrast and weight across hues so darker hues (such as blues)
   * don't overpower high-luminance hues (such as yellows), creating an even ambient field.
   */
  function getPerceptualIntensityMultiplier(
    rgb: number[],
    isDark: boolean,
  ): number {
    const [r, g, b] = rgb;
    // ITU-R BT.709 relative luminance
    const lum = (0.2126 * r + 0.7152 * g + 0.0722 * b) / 255;

    if (!isDark) {
      // Light background: visual weight perceived by the eye scales with contrast from white (1.0 - lum).
      // Target contrast is calibrated to sunny yellow (~0.22 contrast).
      const contrast = Math.max(1.0 - lum, 0.12);
      const targetContrast = 0.22;
      const factor = targetContrast / contrast;
      // Clamp between 0.32 and 1.05 to maintain harmonious, non-dominant presence
      return Math.min(Math.max(factor, 0.32), 1.05);
    } else {
      // Dark background with screen blending: visual weight scales with luminance.
      const contrast = Math.max(lum, 0.15);
      const targetContrast = 0.42;
      const factor = targetContrast / contrast;
      return Math.min(Math.max(factor, 0.45), 1.25);
    }
  }

  const BG_LIGHT = [248, 250, 253];
  const BG_DARK = [8, 11, 19];

  /* Defining the harmonic Lissajous Blob class */
  class AuraBlob {
    index: number;
    phaseX: number;
    phaseY: number;
    phaseScale: number;
    speedX1: number;
    speedX2: number;
    speedY1: number;
    speedY2: number;
    offsetX: number = 0;
    offsetY: number = 0;
    targetOffsetX: number = 0;
    targetOffsetY: number = 0;
    x: number = 0;
    y: number = 0;
    currentRadius: number = 0;

    constructor(index: number, total: number) {
      this.index = index;
      this.phaseX = (index * Math.PI * 2) / total;
      this.phaseY = (index * Math.PI * 1.5) / total + 0.5;
      this.phaseScale = index * 1.2;

      // Incommensurate harmonic speeds for organic, non-repeating drift
      this.speedX1 = 0.00045 + (index % 3) * 0.00015;
      this.speedX2 = 0.00072 + (index % 2) * 0.00018;
      this.speedY1 = 0.00052 + (index % 4) * 0.00012;
      this.speedY2 = 0.00085 + (index % 3) * 0.00014;
    }

    /* Updating particle coordinates and pointer interaction */
    update(
      time: number,
      speedMultiplier: number,
      pointer: { x: number | null; y: number | null },
      enablePointer: boolean,
      width: number,
      height: number,
    ) {
      const t = time * speedMultiplier;

      // Smooth double harmonic paths
      const normalizedX =
        0.5 +
        0.32 * Math.sin(t * this.speedX1 + this.phaseX) +
        0.14 * Math.cos(t * this.speedX2 * 1.4 + this.phaseY);

      const normalizedY =
        0.5 +
        0.32 * Math.cos(t * this.speedY1 + this.phaseY) +
        0.14 * Math.sin(t * this.speedY2 * 1.3 + this.phaseX);

      const posX = normalizedX * width;
      const posY = normalizedY * height;

      // Soft pointer repulsion
      if (enablePointer && pointer.x !== null && pointer.y !== null) {
        const dx = posX - pointer.x;
        const dy = posY - pointer.y;
        const dist = Math.hypot(dx, dy);
        const maxDist = Math.min(width, height) * 0.45;

        if (dist < maxDist && dist > 1) {
          const force = (1 - dist / maxDist) * 120;
          this.targetOffsetX = (dx / dist) * force;
          this.targetOffsetY = (dy / dist) * force;
        } else {
          this.targetOffsetX = 0;
          this.targetOffsetY = 0;
        }
      } else {
        this.targetOffsetX = 0;
        this.targetOffsetY = 0;
      }

      // Smooth spring damping
      this.offsetX += (this.targetOffsetX - this.offsetX) * 0.08;
      this.offsetY += (this.targetOffsetY - this.offsetY) * 0.08;

      this.x = posX + this.offsetX;
      this.y = posY + this.offsetY;

      // Expansive base radius so colors dissolve across viewport boundaries
      const baseRadius = Math.max(width, height) * 0.68;
      const scaleVariation = 0.25 * Math.sin(t * 0.0004 + this.phaseScale);
      this.currentRadius = baseRadius * (1 + scaleVariation);
    }

    /* Rendering the multi-stop atmospheric gradient */
    render(
      ctx: CanvasRenderingContext2D,
      color: number[],
      alpha: number,
      blendMode: GlobalCompositeOperation,
      diffusionScale: number,
    ) {
      const r = this.currentRadius * diffusionScale;
      if (r <= 0) return;

      const gradient = ctx.createRadialGradient(
        this.x,
        this.y,
        0,
        this.x,
        this.y,
        r,
      );
      const [red, green, blue] = color;

      // 6-step exponential decay curve
      gradient.addColorStop(
        0,
        `rgba(${red}, ${green}, ${blue}, ${alpha * 0.85})`,
      );
      gradient.addColorStop(
        0.2,
        `rgba(${red}, ${green}, ${blue}, ${alpha * 0.65})`,
      );
      gradient.addColorStop(
        0.45,
        `rgba(${red}, ${green}, ${blue}, ${alpha * 0.38})`,
      );
      gradient.addColorStop(
        0.7,
        `rgba(${red}, ${green}, ${blue}, ${alpha * 0.16})`,
      );
      gradient.addColorStop(
        0.88,
        `rgba(${red}, ${green}, ${blue}, ${alpha * 0.04})`,
      );
      gradient.addColorStop(1, `rgba(${red}, ${green}, ${blue}, 0)`);

      ctx.globalCompositeOperation = blendMode;
      ctx.fillStyle = gradient;
      ctx.beginPath();
      ctx.arc(this.x, this.y, r, 0, Math.PI * 2);
      ctx.fill();
    }
  }

  /* Managing canvas state and animation loop */
  let canvasEl: HTMLCanvasElement | undefined = $state();
  let animationFrameId: number;
  let ctx: CanvasRenderingContext2D | null = null;
  let width = 0;
  let height = 0;

  const pointer = { x: null as number | null, y: null as number | null };
  const currentBgColor = [...BG_LIGHT];

  const BLOB_COUNT = 5;
  const blobs = Array.from(
    { length: BLOB_COUNT },
    (_, i) => new AuraBlob(i, BLOB_COUNT),
  );

  // Interpolated blob colors for silky smooth transitions across theme and mode changes
  const currentBlobColors: number[][] = [];

  function resize() {
    if (!canvasEl) return;
    const dpr = Math.min(window.devicePixelRatio || 1, 1.5);
    width = window.innerWidth;
    height = window.innerHeight;

    canvasEl.width = Math.floor(width * dpr);
    canvasEl.height = Math.floor(height * dpr);
    ctx =
      canvasEl.getContext("2d", { alpha: false }) || canvasEl.getContext("2d");
    if (ctx) {
      ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    }
  }

  function handlePointerMove(e: PointerEvent | MouseEvent) {
    if (!interactive) return;
    pointer.x = e.clientX;
    pointer.y = e.clientY;
  }

  function handlePointerLeave() {
    pointer.x = null;
    pointer.y = null;
  }

  function handleTouchMove(e: TouchEvent) {
    if (!interactive || !e.touches.length) return;
    pointer.x = e.touches[0].clientX;
    pointer.y = e.touches[0].clientY;
  }

  /* Executing the animation frame loop */
  function loop(currentTime: number) {
    if (!ctx) {
      animationFrameId = requestAnimationFrame(loop);
      return;
    }

    const targetBg = dark ? BG_DARK : BG_LIGHT;
    const colors = dark ? currentTheme.dark : currentTheme.light;

    // Effective opacity: uses calibrated defaults (25% on light, 15% on dark) if not overridden
    const effectiveOpacity = opacity !== null ? opacity : 0.1;
    const blendMode: GlobalCompositeOperation = dark ? "screen" : "source-over";

    // Interpolate background base tone smoothly
    for (let i = 0; i < 3; i++) {
      currentBgColor[i] += (targetBg[i] - currentBgColor[i]) * 0.05;
    }

    ctx.globalCompositeOperation = "source-over";
    ctx.fillStyle = `rgb(${Math.round(currentBgColor[0])}, ${Math.round(currentBgColor[1])}, ${Math.round(currentBgColor[2])})`;
    ctx.fillRect(0, 0, width, height);

    // Draw all harmonic auras with normalized perceptual luminance and smooth color morphing
    for (let i = 0; i < blobs.length; i++) {
      const blob = blobs[i];
      const target = colors[i % colors.length];

      if (!currentBlobColors[i]) {
        currentBlobColors[i] = [...target];
      } else {
        // Smoothly morph RGB towards target color
        for (let c = 0; c < 3; c++) {
          currentBlobColors[i][c] +=
            (target[c] - currentBlobColors[i][c]) * 0.05;
        }
      }

      const color = currentBlobColors[i];
      const intensityMult = getPerceptualIntensityMultiplier(color, dark);

      blob.update(currentTime, speed, pointer, interactive, width, height);
      blob.render(
        ctx,
        color,
        effectiveOpacity * intensityMult,
        blendMode,
        diffusion,
      );
    }

    animationFrameId = requestAnimationFrame(loop);
  }

  /* Attaching lifecycle hooks */
  onMount(() => {
    resize();
    window.addEventListener("resize", resize);
    window.addEventListener("pointermove", handlePointerMove);
    window.addEventListener("pointerleave", handlePointerLeave);
    window.addEventListener("touchmove", handleTouchMove, { passive: true });
    window.addEventListener("touchend", handlePointerLeave);

    animationFrameId = requestAnimationFrame(loop);
  });

  onDestroy(() => {
    if (typeof window !== "undefined") {
      window.removeEventListener("resize", resize);
      window.removeEventListener("pointermove", handlePointerMove);
      window.removeEventListener("pointerleave", handlePointerLeave);
      window.removeEventListener("touchmove", handleTouchMove);
      window.removeEventListener("touchend", handlePointerLeave);
    }
    if (animationFrameId) {
      cancelAnimationFrame(animationFrameId);
    }
  });
</script>

<div class="aura-container {className}" class:is-dark={dark}>
  <!-- Background Canvas surface -->
  <canvas bind:this={canvasEl} class="aura-canvas"></canvas>

  <!-- Film grain layer to eliminate banding -->
  {#if showNoise}
    <div class="aura-noise" aria-hidden="true"></div>
  {/if}

  <!-- App Content Layer -->
  <div class="aura-content">
    {@render children?.()}
  </div>
</div>

<style>
  .aura-container {
    position: relative;
    width: 100%;
    height: 100vh;
    height: 100dvh;
    min-height: 100vh;
    background-color: #f8fafd;
    transition: background-color 0.8s ease;
    overflow: hidden;
  }

  @media (max-width: 860px) {
    .aura-container {
      height: 100vh;
      height: 100dvh;
      overflow: hidden;
    }
  }

  .aura-container.is-dark {
    background-color: #080b13;
  }

  .aura-canvas {
    position: fixed;
    inset: 0;
    width: 100vw;
    height: 100vh;
    display: block;
    pointer-events: none;
    filter: blur(24px);
    transform: scale(1.06); /* Eliminates blur edge-clipping */
    will-change: transform;
    z-index: 0;
  }

  .aura-noise {
    position: fixed;
    inset: 0;
    width: 100vw;
    height: 100vh;
    pointer-events: none;
    z-index: 1;
    opacity: 0.045;
    background-image: url("data:image/svg+xml,%3Csvg viewBox='0 0 200 200' xmlns='http://www.w3.org/2000/svg'%3E%3Cfilter id='noiseFilter'%3E%3CfeTurbulence type='fractalNoise' baseFrequency='0.85' numOctaves='3' stitchTiles='stitch'/%3E%3C/filter%3E%3Crect width='100%25' height='100%25' filter='url(%23noiseFilter)'/%3E%3C/svg%3E");
  }

  .aura-container.is-dark .aura-noise {
    opacity: 0.07;
  }

  .aura-content {
    position: relative;
    z-index: 2;
    width: 100%;
    height: 100%;
  }
</style>
