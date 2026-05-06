[$brainstorming](/Users/apple/Public/openProject/flashmemory/.agents/skills/brainstorming/SKILL.md) [$research-paper-writer](/Users/apple/.agents/skills/research-paper-writer/SKILL.md) [$research-paper-writing](/Users/apple/.agents/skills/research-paper-writing/SKILL.md)

# Author Notes for `2026-04-23-semiotics-centered-memory-survey-paper.md`

- 日期：2026-04-23
- 状态：author companion notes
- 关系：配套 [2026-04-23-semiotics-centered-memory-survey-paper.md](./2026-04-23-semiotics-centered-memory-survey-paper.md) 的图像提示词与作者自检记录；默认不进入正式 arXiv 投稿正文

## Figure Prompt Bank

### FIG-1 Teaser Figure Prompt

```text
Create an arXiv-style academic teaser figure in clean vector style, white background, minimal ink, no photorealism, no 3D rendering. The figure should visualize a semiotic memory architecture for evolving AI systems. Center the core triad sign-object-interpretant. Around it, show five stacked layers: perceptual buffer, working memory, episodic memory, semantic memory, metamemory. On the right, show a self-play harness with task generator, role allocator, debate/reflection loop, outcome judge, consolidation controller. On the far right, show a semiosphere with multiple regions: code, design, incident, product, human tacit. At the bottom, show FlashMemory as substrate with object graph, multimodal sign index, temporal provenance, retrieval layer. Use restrained academic colors: dark teal, slate blue, muted orange, gray. Include arrows indicating propagation, contestation, consolidation, and canonization. Typography should be clean and highly legible, similar to polished arXiv systems figures.
```

### FIG-2 SMU Schema Prompt

```text
Create an arXiv-style academic diagram for a semiotic memory unit schema. Place a central rounded rectangle labeled Semiotic Memory Unit (SMU). Surround it with clearly labeled nodes: canonical sign, sign variants, object anchors, interpretant, context, provenance, confidence, social status, relations, metrics, timestamps. Use directional arrows to indicate that sign variants and object anchors feed into the interpretant, while provenance and confidence govern social status transitions. Add small callouts for example relation types: supports, contests, refines, supersedes. The figure should be publication-quality, flat vector design, white background, thin lines, no gradients, minimal but elegant color coding.
```

### FIG-3 Semiosphere and Propagation Prompt

```text
Design an arXiv-style systems figure illustrating collective memory as a semiosphere. Show five heterogeneous discourse regions arranged as interconnected zones: Code Region, Design Region, Incident Region, Product Region, Human Tacit Region. In the middle, place a shared object graph. Between regions, draw boundary filters and translation edges. On the side, add a propagation pipeline with four stages: diffusion, compression, drift, stabilization. Visualize contested interpretations as red-orange dashed lines and stable consensus as solid blue-green lines. Style: polished arXiv preprint figure, vector infographic, white background, no clutter, strong visual hierarchy.
```

### FIG-4 Evaluation Matrix Prompt

```text
Create an arXiv-style evaluation roadmap figure for a semiotic memory research agenda. Use a matrix-style layout with four experimental axes: Memory Unit, Memory Layers, Evolution Mechanism, Social Structure. Under each axis show discrete conditions: plain chunk / object-anchored chunk / semiotic memory unit; flat / layered memory; no reflection / single-agent reflection / debate / self-play harness; single agent / human-agent pair / multi-agent cluster / multi-region semiosphere. On the right side, show five metric families as a labeled panel: Semiotic Stability, Interpretive Divergence, Symbol Grounding Strength, Canon Quality, Repair Efficiency. Use clean vector style, white background, muted technical palette, highly legible labels, compact layout.
```

### FIG-COMPOSITE 2x2 Draft Sheet Prompt

```text
Create a single 2x2 composite academic figure sheet in arXiv preprint style, white background, clean vector design, minimal ink, no 3D, no photorealism. Panel A: teaser overview of a semiotic memory architecture with sign-object-interpretant at center, cognitive layers, self-play harness, semiosphere, and FlashMemory substrate. Panel B: Semiotic Memory Unit schema with canonical sign, sign variants, object anchors, interpretant, context, provenance, confidence, social status, relations, metrics, timestamps. Panel C: collective memory semiosphere with regions Code, Design, Incident, Product, Human Tacit, shared object graph, translation edges, and propagation pipeline diffusion/compression/drift/stabilization. Panel D: evaluation roadmap matrix with axes Memory Unit, Memory Layers, Evolution Mechanism, Social Structure, and metric families Semiotic Stability, Interpretive Divergence, Symbol Grounding Strength, Canon Quality, Repair Efficiency. Use restrained academic colors: dark teal, slate blue, muted orange, warm gray. Typography should feel like polished arXiv systems/ML figures. Clearly label panels A, B, C, D.
```

## Self-Review Checklist

- Contribution：本文是否不仅总结了八章内容，而且提出了一个清晰、可辩护的中心命题？
  - 结论：是。中心命题明确为“AI memory 的核心问题是意义稳定性，而非内容缓存”。
- Writing clarity：每个主要 section 是否有单一明确的信息中心？
  - 结论：基本是。Introduction 讲问题重写，Section 3 讲八章综合，Section 5 讲可验证性。
- Experimental strength：是否给出了可接受的实验化入口？
  - 结论：是。Section 5 和 Figure 4 提供了实验矩阵和指标族。
- Evaluation completeness：是否覆盖了单体、群体、传播、制度化四个层次？
  - 结论：是，但未来仍需补 benchmark 设计细节。
- Method design soundness：是否把半概念性主张压到了可以落 spec 的层面？
  - 结论：是。与 [2026-04-23-semiotic-memory-unit-schema-spec.md](./2026-04-23-semiotic-memory-unit-schema-spec.md) 已形成衔接。

## Claim–Evidence Map

| Claim | Evidence | Status |
|---|---|---|
| 当前 AI memory 的核心瓶颈是意义漂移而非单纯遗忘 | 八章综合分析 + semiotic framing + propagation/drift chapter synthesis | supported as position claim |
| sign-object-interpretant 比 chunk 更适合作为长期记忆最小单元 | Chapter 2 的 SMU 结构 + Peirce triad + schema spec 连续性 | supported as design claim |
| 群体记忆必须建模 semiosphere 与翻译边界 | Lotman [6], Nöth [14], Chapters 5–6 | supported |
| self-play harness 是记忆演化的关键机制 | Reflexion [10], CAMEL [11], Generative Agents [9], Chapter 4 synthesis | supported |
| FlashMemory 更适合做 substrate 而非直接做完整 agent 平台 | Chapters 7–8 的架构定位 + object anchoring requirement | supported as systems claim |
| 未来评测应转向 semiotic stability、grounding 和 canon quality | Chapter 8 roadmap + Section 5 experimental synthesis | supported as agenda claim |
