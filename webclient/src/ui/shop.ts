// NPC shop window. Buy: add catalogue items to a cart. Sell: add items from
// your inventory (shops pay half the buy price, like RO).
import { shopMessages, ShopCode, type Connection, type InvItem } from "../net";
import type { ItemInfo } from "../items";
import type { PlayerState, ShopState } from "../state";
import { formatZeny, itemCell, toast } from "./common";
import { GameWindow } from "./window";

const MAX_AMOUNT = 999;
type Mode = "buy" | "sell";

export class ShopWindow {
  readonly window = new GameWindow("shop", "Shop", { x: 330, y: 90, width: 520 });
  private shop: ShopState | null = null;
  private mode: Mode = "buy";
  /** Buy cart: itemId -> amount. Sell cart: inventory index -> amount. */
  private buyCart = new Map<number, number>();
  private sellCart = new Map<number, number>();
  private waiting = false;
  private readonly title = document.createElement("p");
  private readonly tabs = document.createElement("div");
  private readonly leftHeading = document.createElement("h3");
  private readonly rightHeading = document.createElement("h3");
  private readonly catalogue = document.createElement("ul");
  private readonly cartList = document.createElement("ul");
  private readonly summary = document.createElement("div");
  private readonly actionButton = document.createElement("button");

  constructor(
    private readonly state: PlayerState,
    private readonly conn: () => Connection | null,
  ) {
    this.title.className = "shop-title";
    this.tabs.className = "tabs";
    this.tabs.setAttribute("role", "tablist");
    for (const [mode, label] of [["buy", "Buy"], ["sell", "Sell"]] as const) {
      const b = document.createElement("button");
      b.type = "button";
      b.setAttribute("role", "tab");
      b.dataset.mode = mode;
      b.textContent = label;
      b.addEventListener("click", () => {
        this.mode = mode;
        this.render();
      });
      this.tabs.append(b);
    }
    this.catalogue.className = "shop-list";
    this.cartList.className = "shop-list cart";

    const cols = document.createElement("div");
    cols.className = "shop-cols";
    const left = document.createElement("div");
    left.append(this.leftHeading, this.catalogue);
    const right = document.createElement("div");
    right.append(this.rightHeading, this.cartList);
    cols.append(left, right);

    const actions = document.createElement("div");
    actions.className = "shop-actions";
    this.actionButton.type = "button";
    this.actionButton.addEventListener("click", () => this.submit());
    const cancel = document.createElement("button");
    cancel.type = "button";
    cancel.className = "secondary";
    cancel.textContent = "Close";
    cancel.addEventListener("click", () => this.window.close());
    actions.append(cancel, this.actionButton);

    this.summary.className = "shop-summary";
    this.window.body.append(this.title, this.tabs, cols, this.summary, actions);
    this.window.onClose = () => {
      this.shop = null;
      this.buyCart.clear();
      this.sellCart.clear();
    };
    state.subscribe(() => this.window.isOpen && this.render());
  }

  /** Called when the server opens an NPC shop. */
  show(shop: ShopState): void {
    this.shop = shop;
    this.mode = "buy";
    this.buyCart.clear();
    this.sellCart.clear();
    this.waiting = false;
    this.window.open();
    this.render();
  }

  /** Called with the server's answer to our Buy or Sell. */
  result(code: number): void {
    this.waiting = false;
    if (code !== ShopCode.Sold) toast(shopMessages[code] ?? "The trade failed."); // sales get a "Sold for" message
    if (code === ShopCode.OK) this.buyCart.clear();
    if (code === ShopCode.Sold) this.sellCart.clear();
    this.render();
  }

  private priceOf(itemId: number): number {
    return this.shop?.items.find((i) => i.itemId === itemId)?.price ?? 0;
  }

  private render(): void {
    const db = this.state.items;
    if (!this.shop || !db) return;
    this.title.textContent = `${this.shop.npcName}: “What would you like?”`;
    for (const b of this.tabs.querySelectorAll<HTMLButtonElement>("button")) {
      b.setAttribute("aria-selected", String(b.dataset.mode === this.mode));
    }
    if (this.mode === "buy") this.renderBuy();
    else this.renderSell();
  }

  private renderBuy(): void {
    const db = this.state.items!;
    this.leftHeading.textContent = "For sale";
    this.rightHeading.textContent = "Your cart";
    this.catalogue.replaceChildren(
      ...this.shop!.items.map(({ itemId, price }) => {
        const info = db.get(itemId);
        if (!info) return document.createElement("li");
        const li = row(info, formatZeny(price));
        li.append(addButton(info, () => this.setAmount(this.buyCart, itemId, (this.buyCart.get(itemId) ?? 0) + 1)));
        return li;
      }),
    );
    this.renderCart(
      [...this.buyCart].map(([itemId, amount]) => ({ key: itemId, info: db.get(itemId)!, amount, price: this.priceOf(itemId) })),
      this.buyCart,
      MAX_AMOUNT,
    );

    const s = this.state.status;
    const total = [...this.buyCart].reduce((sum, [id, n]) => sum + this.priceOf(id) * n, 0);
    const weight = [...this.buyCart].reduce((sum, [id, n]) => sum + (db.get(id)?.weight ?? 0) * n, 0);
    const zeny = s?.zeny ?? 0;
    const overWeight = s ? s.weight + weight > s.maxWeight : false;
    this.summary.replaceChildren(
      line("Total", formatZeny(total), total > zeny),
      line("You have", formatZeny(zeny)),
      line("Weight after", s ? `${s.weight + weight} / ${s.maxWeight}` : "–", overWeight),
    );
    this.actionButton.textContent = "Buy";
    this.actionButton.disabled = this.waiting || this.buyCart.size === 0 || total > zeny || overWeight;
  }

  private renderSell(): void {
    const db = this.state.items!;
    const sellable = this.state.inventory.filter((it) => it.equip === 0 && db.get(it.itemId));
    // Drop cart lines for items that are gone (e.g. just used).
    for (const index of [...this.sellCart.keys()]) {
      const it = sellable.find((s) => s.index === index);
      if (!it) this.sellCart.delete(index);
      else this.sellCart.set(index, Math.min(this.sellCart.get(index)!, it.amount));
    }

    this.leftHeading.textContent = "Your items";
    this.rightHeading.textContent = "To sell";
    if (sellable.length === 0) {
      this.catalogue.replaceChildren(emptyNote("Nothing to sell. Worn equipment can't be sold."));
    } else {
      this.catalogue.replaceChildren(
        ...sellable.map((it) => {
          const info = db.get(it.itemId)!;
          const li = row(info, `${formatZeny(sellPrice(info))} each`, it.amount);
          const left = it.amount - (this.sellCart.get(it.index) ?? 0);
          const add = addButton(info, () => this.setAmount(this.sellCart, it.index, (this.sellCart.get(it.index) ?? 0) + 1, it.amount));
          add.disabled = left <= 0;
          li.append(add);
          return li;
        }),
      );
    }
    const byIndex = new Map<number, InvItem>(sellable.map((it) => [it.index, it]));
    this.renderCart(
      [...this.sellCart].map(([index, amount]) => {
        const info = db.get(byIndex.get(index)!.itemId)!;
        return { key: index, info, amount, price: sellPrice(info), max: byIndex.get(index)!.amount };
      }),
      this.sellCart,
      MAX_AMOUNT,
    );

    const total = [...this.sellCart].reduce((sum, [index, n]) => sum + sellPrice(db.get(byIndex.get(index)!.itemId)!) * n, 0);
    this.summary.replaceChildren(line("You receive", formatZeny(total)), line("You have", formatZeny(this.state.status?.zeny ?? 0)));
    this.actionButton.textContent = "Sell";
    this.actionButton.disabled = this.waiting || this.sellCart.size === 0;
  }

  private renderCart(
    lines: { key: number; info: ItemInfo; amount: number; price: number; max?: number }[],
    cart: Map<number, number>,
    cap: number,
  ): void {
    if (lines.length === 0) {
      this.cartList.replaceChildren(emptyNote("Click Add on an item to put it here."));
      return;
    }
    this.cartList.replaceChildren(
      ...lines.map(({ key, info, amount, price, max }) => {
        const limit = Math.min(cap, max ?? cap);
        const li = row(info, formatZeny(price * amount));
        const qty = document.createElement("div");
        qty.className = "qty";
        const minus = smallButton("−", `One fewer ${info.name}`, () => this.setAmount(cart, key, amount - 1, limit));
        const input = document.createElement("input");
        input.type = "number";
        input.min = "0";
        input.max = String(limit);
        input.value = String(amount);
        input.setAttribute("aria-label", `${info.name} amount`);
        input.addEventListener("change", () => this.setAmount(cart, key, Number(input.value) || 0, limit));
        const plus = smallButton("+", `One more ${info.name}`, () => this.setAmount(cart, key, amount + 1, limit));
        plus.disabled = amount >= limit;
        qty.append(minus, input, plus);
        li.append(qty);
        return li;
      }),
    );
  }

  private setAmount(cart: Map<number, number>, key: number, amount: number, limit = MAX_AMOUNT): void {
    const n = Math.max(0, Math.min(limit, Math.floor(amount)));
    if (n === 0) cart.delete(key);
    else cart.set(key, n);
    this.render();
  }

  private submit(): void {
    const c = this.conn();
    if (!c || !this.shop) return;
    if (this.mode === "buy" && this.buyCart.size) c.buy(this.shop.npcId, [...this.buyCart]);
    else if (this.mode === "sell" && this.sellCart.size) c.sell(this.shop.npcId, [...this.sellCart]);
    else return;
    this.waiting = true;
    this.render();
  }
}

const sellPrice = (info: ItemInfo) => Math.floor(info.price / 2);

function row(info: ItemInfo, priceText: string, amount = 1): HTMLLIElement {
  const li = document.createElement("li");
  const name = document.createElement("span");
  name.className = "shop-name";
  name.textContent = amount > 1 ? `${info.name} × ${amount}` : info.name;
  const price = document.createElement("span");
  price.className = "shop-price";
  price.textContent = priceText;
  li.append(itemCell(info, amount), name, price);
  return li;
}

function addButton(info: ItemInfo, onClick: () => void): HTMLButtonElement {
  const b = smallButton("Add", `Add ${info.name} to cart`, onClick);
  return b;
}

function emptyNote(text: string): HTMLLIElement {
  const li = document.createElement("li");
  li.className = "muted empty-cart";
  li.textContent = text;
  return li;
}

function smallButton(text: string, label: string, onClick: () => void): HTMLButtonElement {
  const b = document.createElement("button");
  b.type = "button";
  b.className = "small-button";
  b.textContent = text;
  b.setAttribute("aria-label", label);
  b.addEventListener("click", onClick);
  return b;
}

function line(label: string, value: string, bad = false): HTMLElement {
  const el = document.createElement("div");
  el.className = bad ? "kv warn" : "kv";
  const k = document.createElement("span");
  k.textContent = label;
  const v = document.createElement("span");
  v.textContent = value;
  el.append(k, v);
  return el;
}
