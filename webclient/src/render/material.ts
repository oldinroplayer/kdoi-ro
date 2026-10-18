// One shader for ground and models: texture * (ambient + diffuse * N.L), with an
// optional RO lightmap (alpha = baked shadow, rgb = baked coloured light).
import * as THREE from "three";

export interface SceneLight {
  /** Direction towards the light, in three.js world space. */
  direction: THREE.Vector3;
  ambient: THREE.Color;
  diffuse: THREE.Color;
}

const vertexShader = /* glsl */ `
attribute vec2 lmuv;
attribute vec3 tint;
varying vec2 vUv;
varying vec2 vLmUv;
varying vec3 vNormal;
varying vec3 vTint;
void main() {
  vUv = uv;
  vLmUv = lmuv;
  vTint = tint;
  vNormal = normalize(mat3(modelMatrix) * normal);
  gl_Position = projectionMatrix * modelViewMatrix * vec4(position, 1.0);
}
`;

const fragmentShader = /* glsl */ `
uniform sampler2D map;
uniform sampler2D lightmap;
uniform bool useLightmap;
uniform vec3 lightDir;
uniform vec3 ambient;
uniform vec3 diffuse;
uniform float opacity;
uniform float alphaTest;
varying vec2 vUv;
varying vec2 vLmUv;
varying vec3 vNormal;
varying vec3 vTint;
void main() {
  vec4 tex = texture2D(map, vUv);
  if (tex.a < alphaTest) discard;
  vec3 n = normalize(gl_FrontFacing ? vNormal : -vNormal);
  vec3 light = clamp(ambient + diffuse * max(dot(n, lightDir), 0.0), 0.0, 1.0);
  vec3 rgb = tex.rgb * vTint * light;
  if (useLightmap) {
    vec4 lm = texture2D(lightmap, vLmUv);
    rgb = rgb * lm.a + lm.rgb;
  }
  gl_FragColor = vec4(rgb, tex.a * opacity);
  #include <colorspace_fragment>
}
`;

export function createMaterial(
  map: THREE.Texture,
  light: SceneLight,
  opts: { lightmap?: THREE.Texture; opacity?: number; alphaTest?: number } = {},
): THREE.ShaderMaterial {
  const opacity = opts.opacity ?? 1;
  return new THREE.ShaderMaterial({
    vertexShader,
    fragmentShader,
    side: THREE.DoubleSide,
    transparent: opacity < 1,
    depthWrite: opacity >= 1,
    uniforms: {
      map: { value: map },
      lightmap: { value: opts.lightmap ?? null },
      useLightmap: { value: !!opts.lightmap },
      lightDir: { value: light.direction },
      ambient: { value: light.ambient },
      diffuse: { value: light.diffuse },
      opacity: { value: opacity },
      alphaTest: { value: opts.alphaTest ?? 0.5 },
    },
  });
}
