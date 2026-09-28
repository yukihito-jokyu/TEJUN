import { useEffect, useId, useRef } from "react";

export function AIUpdateRipple() {
  const id = useId();
  const svg = useRef<SVGSVGElement>(null);
  const deformation = useRef<SVGAnimateElement>(null);

  useEffect(() => {
    const surface = svg.current?.closest("main");
    if (!surface) return;
    const start = (event: AnimationEvent) => {
      if (event.animationName !== "ai-update-ripple" || event.pseudoElement !== "::after") return;
      const target = event.target;
      if (!(target instanceof HTMLElement)) return;
      target.style.filter = `url(#${id})`;
      deformation.current?.setAttribute(
        "dur",
        getComputedStyle(target, "::after").animationDuration,
      );
      deformation.current?.beginElement();
    };
    const end = (event: AnimationEvent) => {
      if (event.animationName === "ai-update-ripple" && event.target instanceof HTMLElement) {
        event.target.style.removeProperty("filter");
      }
    };
    surface.addEventListener("animationstart", start);
    surface.addEventListener("animationend", end);
    surface.addEventListener("animationcancel", end);
    return () => {
      surface.removeEventListener("animationstart", start);
      surface.removeEventListener("animationend", end);
      surface.removeEventListener("animationcancel", end);
    };
  }, [id]);

  return (
    <svg ref={svg} width="0" height="0" aria-hidden="true" className="ai-update-filter">
      <defs>
        <filter
          id={id}
          x="-10%"
          y="-10%"
          width="120%"
          height="120%"
          colorInterpolationFilters="sRGB"
        >
          <feTurbulence
            type="fractalNoise"
            baseFrequency="0.008 0.025"
            numOctaves="1"
            seed="4"
            result="glass"
          />
          <feDisplacementMap
            in="SourceGraphic"
            in2="glass"
            scale="0"
            xChannelSelector="R"
            yChannelSelector="G"
          >
            <animate
              ref={deformation}
              attributeName="scale"
              values="0;18;0"
              keyTimes="0;0.35;1"
              dur="0.96s"
              begin="indefinite"
              fill="freeze"
            />
          </feDisplacementMap>
        </filter>
      </defs>
    </svg>
  );
}
