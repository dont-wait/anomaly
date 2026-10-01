# Anomaly Admin Console — Desktop Design System

## 1. Purpose

This document defines the visual foundation for the Anomaly Admin Console. It is
derived from the existing Ethereal Neo-Banking consumer design system, but is a
separate admin variant optimized for desktop investigation, dense tables and
operational decision-making.

The admin design system must not change the consumer tokens in
`client/mobile/src/index.css`. The `client/desktop/` app owns its own CSS entry point and
may reuse the brand colors and Be Vietnam Pro font family.

## 2. Design Principles

### 2.1 Trust through clarity

Admin users make decisions about risk, money and identity. Information must be
easy to scan, compare and verify. Decorative effects must never reduce contrast
or hide state.

### 2.2 Data density with breathing room

Use compact table rows and metadata, but keep clear group boundaries. Prefer a
well-structured table over a collection of oversized cards when the user needs
to compare records.

### 2.3 Action follows evidence

Destructive or sensitive actions appear next to the evidence that supports them.
Review, approve, reject, escalate and replay actions require visible context and
an explicit confirmation step where appropriate.

### 2.4 Eventual consistency is visible

Query data can lag behind a command because the read model is projected after an
event is written. The UI must show update time and stale state instead of
pretending that a command has immediately changed every screen.

### 2.5 Keep the brand, change the density

Retain AnomalyBank's navy and violet identity. Reduce the consumer app's use of
large pills, heavy blur and oversized empty space for admin surfaces.

## 3. Visual Direction

### 3.1 Brand relationship

| Consumer system | Admin variant |
|-----------------|---------------|
| Be Vietnam Pro | Be Vietnam Pro |
| Navy primary | Same navy family, used more selectively |
| Violet accent | Same violet family for focus and active states |
| Glassmorphism | Limited to login, topbar or selected emphasis |
| Large rounded cards | Smaller operational cards and flat table surfaces |
| Mobile-first spacing | Desktop grid and compact data rhythm |

### 3.2 Overall mood

The admin console should feel:

- Calm under pressure.
- Precise rather than playful.
- Premium but operational.
- Secure without looking militarized.
- Dense enough for investigation, never visually noisy.

## 4. Viewport and Layout

### 4.1 Supported sizes

| Viewport | Behavior |
|----------|----------|
| `1280x800` | Minimum supported desktop layout; tables may scroll horizontally within content |
| `1440x900` | Primary design and review viewport |
| `1920x1080` | Wider content canvas; do not stretch table cells excessively |
| `<1280px` | Preserve shell; collapse optional panels and allow horizontal table scroll |

Mobile layout is not a target for the admin app. If the window becomes narrow,
the interface should degrade into a usable tablet-like layout rather than mimic
the consumer mobile navigation.

### 4.2 Application shell

```text
┌──────────────────────────────────────────────────────────────────────┐
│ Sidebar 256px │ Topbar 64px                                          │
│               ├───────────────────────────────────────────────────────┤
│               │ Breadcrumb / page title / actions                     │
│               ├───────────────────────────────────────────────────────┤
│               │                                                       │
│               │ Main content, 12-column grid                          │
│               │                                                       │
└───────────────┴───────────────────────────────────────────────────────┘
```

### 4.3 Sidebar

- Expanded width: `256px`.
- Collapsed width: `72px`.
- Fixed to the left of the viewport.
- Brand block at the top.
- Navigation groups with clear active state.
- Alert badge may show `999+` as the maximum visible count.
- Staff profile and session actions at the bottom.
- Collapsed mode keeps icon tooltips and active indicator.

Navigation groups:

1. Tổng quan rủi ro.
2. Điều tra.
3. Khoản vay.
4. KYC.
5. Báo cáo.
6. Quản trị, visible only when the staff member has an applicable permission.

### 4.4 Topbar

- Height: `64px`.
- Left: breadcrumb and page context.
- Center: optional global search.
- Right: freshness indicator, notifications, theme/session menu.
- Bottom border: subtle `outline-variant`.
- Keep the topbar visually quiet; alerts and data should dominate the page.

### 4.5 Content canvas

- Horizontal padding: `32px` at 1280px, up to `40px` at 1440px+.
- Maximum content width: `1360px`.
- Grid: 12 columns.
- Standard gutter: `24px`.
- Section gap: `24px`.
- Page header bottom gap: `24px`.

### 4.6 Right context drawer

- Width: `440px` at 1440px.
- Maximum width: `min(480px, 42vw)`.
- Use for alert context, selected event, quick account summary and action review.
- Keep primary investigation workspaces in the page, not hidden inside a drawer.
- Drawer has a clear title, close button, scrollable body and sticky footer when
  actions exist.

## 5. Design Tokens

The following tokens are the design contract. The implementation may map these
to Tailwind theme variables or CSS custom properties.

### 5.1 Color tokens

#### Brand

| Token | Value | Use |
|-------|-------|-----|
| `admin-primary` | `#1E3A8A` | Primary navigation, primary buttons, active controls |
| `admin-primary-strong` | `#172554` | Hover, pressed and high-contrast navy |
| `admin-primary-soft` | `#E8EEFF` | Selected row, soft info surface |
| `admin-accent` | `#8B5CF6` | Focus, chart accent, secondary emphasis |
| `admin-accent-soft` | `#F1ECFF` | Accent background |

#### Surfaces

| Token | Value | Use |
|-------|-------|-----|
| `admin-background` | `#F6F7FB` | Application canvas |
| `admin-surface` | `#FFFFFF` | Cards, panels and table surface |
| `admin-surface-subtle` | `#F1F3F8` | Secondary panels and input backgrounds |
| `admin-surface-hover` | `#F8F9FC` | Hover row and hover surface |
| `admin-surface-selected` | `#EEF2FF` | Selected row or selected filter |
| `admin-surface-inverse` | `#101828` | Inverse banner and dark data panel |

#### Text and borders

| Token | Value | Use |
|-------|-------|-----|
| `admin-text` | `#101828` | Primary text |
| `admin-text-secondary` | `#475467` | Supporting text |
| `admin-text-muted` | `#667085` | Metadata, timestamps and placeholders |
| `admin-text-disabled` | `#98A2B3` | Disabled text |
| `admin-border` | `#D0D5DD` | Default borders |
| `admin-border-subtle` | `#EAECF0` | Dividers and table row borders |
| `admin-border-strong` | `#98A2B3` | Focus-adjacent or emphasized border |

#### Functional colors

| Semantic | Main | Soft container | Use |
|----------|------|----------------|-----|
| Success | `#15803D` | `#ECFDF3` | Completed, verified, paid |
| Info | `#2563EB` | `#EFF6FF` | Informational and historical |
| Warning | `#B54708` | `#FFFAEB` | Attention, pending, stale |
| Danger | `#B42318` | `#FEF3F2` | Error, rejected, destructive |
| Neutral | `#475467` | `#F2F4F7` | Draft, archived, inactive |

### 5.2 Risk and severity colors

| Level | Main | Soft container | Text label |
|-------|------|----------------|------------|
| Low | `#2563EB` | `#EFF6FF` | Thấp |
| Medium | `#B54708` | `#FFFAEB` | Trung bình |
| High | `#C2410C` | `#FFF7ED` | Cao |
| Critical | `#B42318` | `#FEF3F2` | Nghiêm trọng |

Risk score ranges:

| Score | Level |
|-------|-------|
| `0–24` | Low |
| `25–49` | Medium |
| `50–74` | High |
| `75–100` | Critical |

The backend may provide a level directly. The UI must prefer the backend level
over calculating a different one.

### 5.3 Data visualization palette

Use a stable palette so a category does not change color across screens.

| Token | Value | Suggested use |
|-------|-------|---------------|
| `chart-blue` | `#2563EB` | Normal volume, low risk |
| `chart-violet` | `#7C3AED` | Secondary series |
| `chart-cyan` | `#0891B2` | Incoming/verified |
| `chart-green` | `#059669` | Positive/paid/resolved |
| `chart-amber` | `#D97706` | Pending/medium risk |
| `chart-orange` | `#EA580C` | High risk |
| `chart-red` | `#DC2626` | Critical/failed |
| `chart-slate` | `#64748B` | Baseline/unknown |

Charts must not rely on color alone. Include labels, legends, patterns or
tooltips and provide a text/table fallback.

### 5.4 Typography

Font family:

```css
font-family: "Be Vietnam Pro", system-ui, sans-serif;
```

| Token | Size / line height | Weight | Use |
|-------|--------------------|--------|-----|
| `admin-display` | `32px / 40px` | 700 | Login/display emphasis only |
| `admin-page-title` | `26px / 34px` | 700 | Page title |
| `admin-section-title` | `18px / 26px` | 600 | Panel and section title |
| `admin-title` | `16px / 24px` | 600 | Card/title text |
| `admin-body` | `14px / 20px` | 400 | Default content |
| `admin-table` | `13px / 20px` | 400–500 | Table cells |
| `admin-label` | `12px / 16px` | 600 | Labels and metadata headers |
| `admin-caption` | `11px / 16px` | 500 | Supporting metadata only |
| `admin-number-xl` | `28px / 36px` | 700 | KPI number |
| `admin-number` | `16px / 24px` | 600 | Monetary table value |

Use `font-variant-numeric: tabular-nums` for money, counts, percentages,
timestamps and revision numbers.

### 5.5 Spacing

Use a 4px base with common 8px rhythm:

| Token | Value | Use |
|-------|-------|-----|
| `space-1` | `4px` | Icon/text micro gap |
| `space-2` | `8px` | Compact control gap |
| `space-3` | `12px` | Field inner spacing |
| `space-4` | `16px` | Default component gap |
| `space-5` | `20px` | Table/card grouping |
| `space-6` | `24px` | Section gap |
| `space-8` | `32px` | Page/content padding |
| `space-10` | `40px` | Large page separation |

### 5.6 Radius

| Token | Value | Use |
|-------|-------|-----|
| `radius-sm` | `6px` | Inputs, compact badges |
| `radius-md` | `8px` | Buttons, cards, table containers |
| `radius-lg` | `12px` | Large cards, drawers |
| `radius-xl` | `16px` | Login panel or featured visual |
| `radius-full` | `9999px` | Pills, avatars, status chips only |

Avoid full-pill buttons for every action. Use pills for compact status/filter
controls; use 8px buttons for standard admin actions.

### 5.7 Elevation

Admin surfaces should use borders before shadows.

| Level | Style | Use |
|-------|-------|-----|
| `flat` | Border only | Tables, regular cards |
| `raised` | `0 4px 16px rgba(16,24,40,.06)` | Dropdowns, drawers |
| `floating` | `0 16px 40px rgba(16,24,40,.12)` | Modal and command palette |
| `glass` | Limited translucent blur | Login/topbar emphasis only |

## 6. Core Components

### 6.1 Buttons

Variants:

- Primary: navy background, white text.
- Secondary: white surface, border, navy text.
- Tertiary: transparent, secondary text.
- Danger: danger background for destructive final action.
- Icon button: square 36px or 40px with tooltip.

Rules:

- Standard height: `40px`.
- Compact height: `32px` for table actions.
- Large height: `44px` for primary page action.
- Every button has hover, pressed, focus-visible and disabled states.
- Loading button keeps its width and replaces label with spinner plus accessible
  status text.

### 6.2 Inputs and filters

- Standard height: `40px`.
- Compact table filter height: `36px`.
- Border is visible even on white background.
- Focus uses a 2px accent ring with sufficient contrast.
- Error appears below the field, not only as a red border.
- Filter chips show active value and a clear action.
- Date ranges display timezone or use the app's configured timezone label.

### 6.3 Stat card

Structure:

```text
Label
Value + unit
Trend / comparison
Optional context timestamp
```

Rules:

- Never use a large decorative illustration inside an operational KPI card.
- Trend includes text such as `+12,5%` and `Tăng`, not color alone.
- Critical KPI may use a subtle left border or top accent, not a full red card.

### 6.4 Data table

Structure:

```text
Table header
 ├── title/result count
 ├── search/filter toolbar
 ├── optional bulk action bar
 └── table
       ├── sticky header
       ├── selectable rows where needed
       ├── status/amount/date cells
       └── cursor pagination
```

Rules:

- Header height: `44px`.
- Body row height: `56px` default, `48px` compact.
- Row border: `admin-border-subtle`.
- Hover: `admin-surface-hover`.
- Selected: `admin-surface-selected` plus visible checkbox state.
- Long text truncates with tooltip; IDs use monospace only when helpful.
- Empty table has an explanation and one relevant next action.
- Table actions stay in the final column and do not trigger row navigation.

Required states:

- Loading skeleton rows.
- No results after filter.
- No records yet.
- Request error with retry.
- Permission-limited fields.
- Stale data banner.

### 6.5 Status and severity badge

Every badge contains a text label. Color and icon are supplementary.

Examples:

- `Mới`, `Đang điều tra`, `Đã xử lý`.
- `Thấp`, `Trung bình`, `Cao`, `Nghiêm trọng`.
- `Nháp`, `Đang hoạt động`, `Đã lưu trữ`.
- `Đạt`, `Cần xem xét`, `Từ chối`.

Badge shape: `radius-full`, height `24px`, horizontal padding `8px`.

### 6.6 Timeline

Use timeline for alert history, audit events and loan decisions.

Each item can contain:

- Timestamp.
- Event/action label.
- Actor.
- Short summary.
- Revision or reference ID.
- Expandable detail.

Current selected event uses accent line and subtle accent background. Historical
events must be visually distinct from pending actions.

### 6.7 Drawer

- Scrim opacity: 24–36% depending on context.
- Width: 440px by default.
- Header remains visible while content scrolls.
- Footer is sticky only when actions are available.
- Escape closes the drawer unless there is unsaved form data.
- Focus moves into the drawer and returns to the trigger on close.

### 6.8 Modal and confirmation dialog

Use a modal for decisions that need explicit confirmation, not for normal detail
reading.

Required confirmation content:

- What will happen.
- Target record.
- Whether the action is reversible.
- Required reason field for sensitive actions.
- Cancel and final action with distinct emphasis.

Replay, reject, disburse, freeze, resolve-critical-alert and export-sensitive-data
must use this pattern.

### 6.9 Charts

Chart selection:

| Question | Chart |
|----------|-------|
| Risk changes over time | Line or area chart |
| Severity volume by period | Stacked area/bar |
| Alert type comparison | Horizontal bar |
| Small trend in a KPI | Sparkline |
| Portfolio composition | Donut only for a small number of categories |

Rules:

- Show unit and time range near the chart title.
- Tooltip includes exact value and timestamp/category.
- Do not use 3D, gradients that hide values or excessive animation.
- Respect `prefers-reduced-motion`.
- Provide a data table or text summary for accessibility.

### 6.10 Diff and payload viewer

- Diff uses left/right or before/after columns.
- Added values use success tint; removed values use danger tint; changed values
  use warning tint.
- JSON is collapsed by default for large payloads.
- Sensitive keys are masked and cannot be revealed without explicit permission.
- Never render password, token or raw media URL values.

## 7. Screen Composition Rules

### 7.1 Monitoring dashboard

```text
Page header + range/refresh
KPI row
Risk trend + severity distribution
Alert type distribution + top risky accounts
Recent alert feed
```

The first viewport should answer:

1. Is risk increasing?
2. How many critical alerts are open?
3. Which accounts need attention?
4. What changed most recently?

### 7.2 Alert queue

Prioritize scan speed:

- Severity and risk score near the left.
- Account and alert type in the center.
- Time and status near the right.
- Actions at the far right.
- Filter state remains visible above the table.

### 7.3 Alert detail

Keep evidence and action in the same visual frame:

- Summary and risk factors at the top.
- Related transactions and timeline in the main column.
- Triage actions in a sticky side panel.

### 7.4 Investigation workspace

Use a three-pane layout at 1440px:

- Account context: 240–280px.
- Event timeline: flexible center.
- Event detail/diff: 360–440px.

At 1280px, event detail may become a drawer or tab panel instead of a permanent
third pane.

### 7.5 Loan review

- Customer and risk context remain visible while reviewing.
- Checklist uses clear pass/pending/fail states.
- Approve/reject actions stay in a sticky footer or right panel.
- Do not hide approval conditions inside a secondary modal.

### 7.6 KYC comparison

- Identity front/back media must be comparable at the same scale.
- OCR and declared fields use aligned rows.
- Confidence scores need explanatory labels.
- Media failure must preserve the rest of the review context.

## 8. Freshness and Command Feedback

### 8.1 Query freshness

Show a compact freshness indicator when the response contains update metadata:

```text
● Cập nhật 12 giây trước
```

Stale state:

```text
! Dữ liệu có thể chưa phản ánh thay đổi mới nhất. [Làm mới]
```

### 8.2 After command

Do not optimistically update unrelated read models. After a command:

1. Disable the submitted action while pending.
2. Show a success toast that the command was accepted.
3. Refresh the current resource/query.
4. If the new state is not available yet, show `Đang cập nhật...`.
5. Keep the user on the same context where possible.

### 8.3 Async job

Replay and large export use a job state pattern:

```text
Queued → Running → Succeeded
                 ├→ Failed
                 └→ Cancelled
```

The UI shows progress, last update, retry/cancel availability and a link back
to the affected resource.

## 9. Accessibility

- Meet WCAG AA contrast for normal text and controls.
- Do not use color as the only status signal.
- Every icon-only button has an accessible label and tooltip.
- Keyboard focus is visible with at least a 2px outline.
- Dialogs and drawers trap focus while open and restore focus when closed.
- Tables use proper headers and support keyboard row/action navigation.
- Charts include a text summary or table fallback.
- Motion respects `prefers-reduced-motion`.
- Vietnamese text must not clip diacritics at any supported size.
- Error messages are associated with their input fields.

## 10. Responsive Behavior

### At 1440px and above

- Full sidebar.
- Three-pane investigation workspace where applicable.
- Dashboard uses full 12-column layout.
- Tables show all priority columns.

### At 1280px

- Full or compact sidebar depending on content width.
- Investigation detail can collapse into a drawer.
- Optional table columns hide behind column visibility control.
- Charts retain readable labels by reducing tick density.

### Below 1280px

- Collapse sidebar to icon rail.
- Stack secondary dashboard widgets.
- Allow horizontal scroll inside tables.
- Convert permanent detail panes to drawers or tabs.
- Never reduce body text below 12px to fit more content.

## 11. Implementation Mapping

The admin app owns an independent CSS entry point:

```text
client/desktop/src/index.css
```

The first implementation should define the tokens in this document before page
components. Suggested token groups:

```text
--admin-color-*
--admin-font-*
--admin-space-*
--admin-radius-*
--admin-shadow-*
--admin-chart-*
```

Do not import consumer feature CSS into admin screens. Reuse only explicitly
portable utilities or assets after checking that they do not encode mobile
layout assumptions such as `max-w-md`, bottom navigation or bottom sheets.

## 12. Phase 1 Acceptance Criteria

- Admin visual direction is distinct from, but recognizable as, AnomalyBank.
- Admin uses opaque, readable surfaces for operational data.
- Sidebar, topbar, content grid and drawer dimensions are defined.
- Brand, surface, text, border, functional, severity and chart tokens are defined.
- Typography includes table, label, KPI and tabular-number rules.
- Core components have visual and interaction states.
- Loading, empty, error, stale and permission-denied patterns are defined.
- Dashboard, alert queue, alert detail, investigation and loan review share the
  same visual language.
- Accessibility and 1280/1440/1920 viewport behavior are documented.
- The design can be implemented without modifying the consumer app tokens.

## 13. Next Phase

After this document is reviewed, create the separate Stitch design system
variant and generate the first six desktop prototype screens:

1. Admin login.
2. Anomaly monitoring dashboard.
3. Alert queue.
4. Alert detail.
5. Account investigation/event timeline.
6. Loan application review.
