// RO's emoticons: index = action in sprite/이팩트/emotion.act (the 2004
// client's 47), typed as chat commands like "/!" or picked with Alt+1..0.

/** Chat command -> emotion number. */
export const EMOTE_COMMANDS: Record<string, number> = {
  "/!": 0, "/?": 1, "/ho": 2, "/lv": 3, "/swt": 4, "/ic": 5, "/an": 6, "/ag": 7, "/$": 8, "/...": 9,
  "/scissors": 10, "/rock": 11, "/paper": 12, "/korea": 13, "/lv2": 14, "/thx": 15, "/wah": 16, "/sry": 17,
  "/heh": 18, "/swt2": 19, "/hmm": 20, "/no1": 21, "/??": 22, "/omg": 23, "/oh": 24, "/x": 25, "/hlp": 26,
  "/go": 27, "/sob": 28, "/gg": 29, "/kis": 30, "/kis2": 31, "/pif": 32, "/ok": 33, "/mute": 34,
  "/indonesia": 35, "/bzz": 36, "/rice": 37, "/awsm": 38, "/meh": 39, "/shy": 40, "/pat": 41, "/mp": 42,
  "/slur": 43, "/com": 44, "/yawn": 45, "/grat": 46,
};

/** Alt+1..Alt+0, as in RO's default emotion shortcuts. */
export const EMOTE_HOTKEYS = [0, 1, 2, 3, 4, 5, 6, 7, 8, 9];

/** A short list for /help. */
export const EMOTE_HELP = "/! /? /ho /lv /swt /ic /an /ag /$ /... /thx /wah /sry /heh /hmm /omg /gg /ok /sob /kis /grat (Alt+1–0)";
