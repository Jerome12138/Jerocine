# Web 样式架构：两段式层叠与硬约定

> 建立于 2026-11-06 —— 一天之内连炸三次「同特异性覆盖」事故后固化。
> 配套：`CLAUDE.md`「Web CSS 层叠约定」（速查版）、`web/docs/manage-ui-conventions.md`（管理端细则）、
> `web/src/assets/styles/style-guards.spec.ts`（防呆断言）。

---

## 1. 加载顺序（唯一事实来源）

`web/src/main.ts` 的 import 顺序即注入顺序：

```
virtual:uno.css   （UnoCSS 工具类 + shortcuts）
reset.css
theme.css         （设计令牌 + 全局元素样式：:root / :focus-visible / .jc-hotarea / .jc-scrollauto …）
tv-cards.css      （TV 词汇类）
auth-tv.css       （认证页 TV 共享语法）
compat.css        （老内核回退，必须最后）
```

**外加一段**：各组件的 `<style>`（无论 `scoped` 与否）随其 chunk **在运行时注入**，
因此实际位置**始终排在上述文件之后**。

## 2. 唯一法则与三段优先级

> **同特异性下，后写覆盖先写。**

于是（特异性相同时）优先级为：

```
组件 <style>   >   theme.css   >   uno.css / reset.css
```

`scoped` 会给选择器尾部加 `[data-v-xxx]`，把特异性抬高一档
（`.a[data-v-x]` (0,2,0) > `.a` (0,1,0)），所以组件内的样式通常**更容易压过**全局。

## 3. 三条硬约定

### 3.1 `theme.css` 里「会被工具类使用的属性」必须用 `:where()` 归零

`theme.css` 晚于 `uno.css`，若它给某个**也会被工具类设置**的属性（`position` / `outline` /
`box-shadow` / `overflow` / `display` …）写了裸类规则，就会**静默压掉工具类**。

```css
/* ✅ 正确：特异性归零，工具类仍可覆盖 */
:where(.jc-hotarea) { position: relative; }

/* ❌ 错误：会压掉 .absolute，元素退回文档流 */
.jc-hotarea { position: relative; }
```

### 3.2 组件里不要写「否定型」规则去盖全局通用类

组件样式晚于 `theme.css` ⇒ 组件里一句 `.x::-webkit-scrollbar { display: none }`
就能把 `theme.css` 的通用类（如 `.jc-scrollauto` 的显形规则）**盖回隐藏**。

要改通用类的行为，请改**通用类本身**（或加带状态的选择器），不要在组件里"就地打死"。

### 3.3 聚焦样式分两处；「蓝框」有**两个**来源

- 全局 `:focus-visible { outline: … }`（theme.css）—— 只服务键盘可达性，**非表单控件保留**。
- 表单控件统由 `.jc-field` 接管（`ManageInput` / `ManageTextarea` / `ManageSelect` / `BasePasswordField`
  的根节点都挂它）：
  ```css
  .jc-field:focus, .jc-field:focus-visible, .jc-field:focus-within {
    outline: none;
    box-shadow: none;   /* ← 别漏！focus:shadow-focus 是青色 3px 环，第二个蓝框来源 */
  }
  ```
- 聚焦语言＝**紫色描边** `--jc-border-ctrl-focus: rgba(155, 73, 231, 0.55)`（与首页搜索框一致）。

## 4. 决策表：这条样式该写在哪？

| 场景 | 写在哪 | 注意 |
|---|---|---|
| 设计令牌（颜色/尺寸/层级） | `theme.css` 的 `:root` / 端媒体查询 / `[data-mode='tv']` | 端差异只在这里出现 |
| 跨组件复用的通用类（`.jc-hotarea` / `.jc-scrollauto`） | `theme.css` | §3.1：涉及工具类属性一律 `:where()` |
| 某个组件的内部样式 | 该组件 `<style scoped>` | §3.2：不要写"否定型"规则盖通用类 |
| 页面级微调某组件 | 页面 `<style scoped>` + `:deep()` | 优先改组件，别在页面"打补丁" |
| 需要覆盖工具类 | 提高特异性（`scoped` 自带 `[data-v]`）或改工具类本身 | 不要靠"写在后面"赌顺序 |

## 5. 三次事故存档（2026-11-06）

| # | 症状 | 根因 | 修法 | commit |
|---|---|---|---|---|
| 1 | 卡角删除钮(×) 漂到左下 / 收藏页整块消失 | `.jc-hotarea{position:relative}`（theme.css）压掉 UnoCSS `.absolute` → 元素回到文档流，被 `overflow-hidden` 容器裁掉 | `:where(.jc-hotarea)` 归零 + 组件补 scoped `.jc-corner-remove{position:absolute}` 兜老内核 | `1dc235e` |
| 2 | 管理后台点进输入框/选择框冒青色粗框 | ① 全局 `:focus-visible{outline:青}` 压掉控件 `.outline-none`；② `focus:shadow-focus` 的青色 3px 环（首轮只修了①） | `.jc-field` 同清 `outline`+`box-shadow`；令牌改品牌紫 | `3c8dca7`、`f778ff6` |
| 3 | 滚动条永远看不见（要求是"滚动时显形"） | 组件内 `.jc-source-tabs{scrollbar-width:none}` + `::-webkit-scrollbar{display:none}` 晚于 theme.css 注入，盖掉 `.jc-scrollauto`；叠加 `none↔thin` 切换在部分内核不生效 | 删组件内隐藏写法 + 改「常占位 thin + 拇指透明↔可见」 | `f778ff6` |

> 三次都满足同一条法则：**同特异性、后来者胜**。所以本仓改样式时，先问「这个属性会不会也被工具类
> 或通用类设置」，再决定写在哪、要不要 `:where()`。

## 6. 防呆

`web/src/assets/styles/style-guards.spec.ts` 对以下不变量做**静态文本断言**（读文件、剥注释、再匹配）：

1. `.jc-hotarea` 的 `position` 必须写在 `:where(...)` 里；
2. `.jc-field:focus*` 必须**同时**含 `outline: none` 与 `box-shadow: none`；
3. `--jc-border-ctrl-focus` 必须是品牌紫（`155, 73, 231`）；
4. `.jc-scrollauto` 必须是「常占位 `thin` + 透明拇指」；**`overscroll-behavior` 不得写在裸 `.jc-scrollauto` 上**，
   只能挂在 `.is-scrollable` + 轴向修饰（`jc-scroll-x` / `jc-scroll-y`）上，且另一轴留 `auto`；
5. 选源条（`EpisodeTabs.vue`）不得再自行 `scrollbar-width: none` / `::-webkit-scrollbar{display:none}`。

**新增防呆**：往里加一条 `it(...)` 即可 —— 典型形态是「读某个文件 → 断言含 / 不含某段文本」。
断言前务必先 `stripComments()`，否则会被"注释里提到该写法"误伤（首版就栽过）。

## 7. 相关文档

- `CLAUDE.md`「Web CSS 层叠约定」—— 给 agent 的速查版
- `web/docs/manage-ui-conventions.md` —— 管理端组件与滚动条细则
- `docs/design/design-tokens.md` —— 令牌分级
- `docs/specs/manage-controls-spec.md` —— 控件尺寸三档
