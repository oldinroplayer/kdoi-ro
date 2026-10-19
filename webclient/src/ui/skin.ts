// The 2004 client's interface bitmaps (texture/유저인터페이스/), loaded once
// and handed to the stylesheet as CSS variables (--ro-title-mid: url(...)),
// so windows, buttons and text boxes are drawn with the real pieces. Magenta
// is made transparent, as the client does. Until they load (or if the files
// are missing) the stylesheet's plain colours stand in.
import { loadKeyedCanvas } from "../assets";

const BASIC = "texture/유저인터페이스/basic_interface/";
const ROOT = "texture/유저인터페이스/";

/** CSS variable name -> bitmap. */
const PIECES: Record<string, string> = {
  // Window title bar (17px tall) and its buttons.
  "--ro-title-left": BASIC + "titlebar_left.bmp",
  "--ro-title-mid": BASIC + "titlebar_mid.bmp",
  "--ro-title-right": BASIC + "titlebar_right.bmp",
  "--ro-close": BASIC + "sys_close_off.bmp",
  "--ro-close-on": BASIC + "sys_close_on.bmp",
  // The Basic Info window's minimize and restore buttons.
  "--ro-mini": BASIC + "sys_mini_off.bmp",
  "--ro-mini-on": BASIC + "sys_mini_on.bmp",
  "--ro-base": BASIC + "sys_base_off.bmp",
  "--ro-base-on": BASIC + "sys_base_on.bmp",
  // Window bottom bar (21px, faint stripes).
  "--ro-bar-left": BASIC + "btnbar_left.bmp",
  "--ro-bar-mid": BASIC + "btnbar_mid.bmp",
  "--ro-bar-right": BASIC + "btnbar_right.bmp",
  // Buttons: a rounded white pill, 20px tall, in three slices.
  "--ro-btn-left": BASIC + "mesbtn_left.bmp",
  "--ro-btn-mid": BASIC + "mesbtn_mid.bmp",
  "--ro-btn-right": BASIC + "mesbtn_right.bmp",
  // Text boxes, 22px tall, in three slices.
  "--ro-txt-left": BASIC + "txtbox_l.bmp",
  "--ro-txt-mid": BASIC + "txtbox_m.bmp",
  "--ro-txt-right": BASIC + "txtbox_r.bmp",
  // The vending stall sign's shop icon.
  "--ro-shop": ROOT + "shop.bmp",
  // Check boxes.
  "--ro-check-off": ROOT + "checkbox_0.bmp",
  "--ro-check-on": ROOT + "checkbox_1.bmp",
};

let loaded: Promise<void> | null = null;

/** Loads the interface bitmaps into CSS variables on <html> (once). */
export function loadSkin(): Promise<void> {
  loaded ??= (async () => {
    const root = document.documentElement.style;
    await Promise.all(
      Object.entries(PIECES).map(async ([name, path]) => {
        try {
          const canvas = await loadKeyedCanvas(path);
          if (canvas) root.setProperty(name, `url("${canvas.toDataURL()}")`);
        } catch (err) {
          console.warn("interface bitmap missing", path, err);
        }
      }),
    );
    document.documentElement.classList.add("ro-skin");
  })();
  return loaded;
}
