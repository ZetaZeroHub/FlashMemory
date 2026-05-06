# Toward a Semiotic Memory Architecture for Evolving AI Systems

> arXiv-style survey-position preprint draft  
> Date: 2026-04-23  
> This manuscript consolidates the eight-part memory research package in [2026-04-22-memory-01-core-thesis.md](./2026-04-22-memory-01-core-thesis.md) through [2026-04-22-memory-08-experiments-metrics-roadmap.md](./2026-04-22-memory-08-experiments-metrics-roadmap.md).

## Abstract

Large language model agents have made rapid progress in retrieval, planning, and tool use, yet most current memory architectures still treat memory as a storage-and-retrieval problem rather than a problem of meaning formation, semantic drift, and collective stabilization. This paper consolidates eight chapters of a broader research program into an arXiv-style survey-position manuscript centered on semiotics. The main claim is that evolving AI memory should be modeled as a semiotic order rather than a text cache: a memory item is not merely a chunk of content, but a dynamically maintained relation among sign, object, and interpretant. Building on this claim, the paper uses cognitive psychology to specify internal layering, game theory to explain competition and update, social diffusion theory to explain propagation and institutionalization, and computational linguistics to operationalize semantic drift. The resulting framework spans five levels, from semiotic memory units and cognitive layers to self-play harnesses, semiospheric collective memory, propagation-aware auditing, and FlashMemory as a substrate for object anchoring and temporal provenance. The paper argues that future AI memory research should shift from maximizing recall toward optimizing semiotic stability, interpretive divergence management, symbol grounding strength, canon quality, and repair efficiency. Rather than merely summarizing the eight chapters, the manuscript reorganizes them into a preprint-ready problem statement, architecture, and evaluation agenda for evolving memory systems.

## Keywords

semiotic memory, AI agents, memory architecture, semiosphere, self-play harness, FlashMemory, semantic drift, collective memory

> [Figure 1 Placeholder]
> Teaser figure: a panoramic overview from `sign-object-interpretant` to `individual memory -> self-play harness -> semiosphere -> FlashMemory substrate`.
> Use `FIG-1` in [2026-04-23-semiotics-centered-memory-survey-paper-author-notes.md](./2026-04-23-semiotics-centered-memory-survey-paper-author-notes.md).

## 1. Introduction

The dominant paradigm for AI memory is still too narrow. In most existing systems, memory is treated as an auxiliary buffer that stores observations, summaries, or vectorized chunks for later retrieval. This paradigm has produced useful engineering gains, but it does not explain why long-horizon agents routinely suffer from semantic drift, sloganized summaries, unstable norms, and conflicting interpretations across agents and modalities. The deeper bottleneck is not simply that systems forget. It is that they fail to preserve the operational meaning of what they remember.

This paper argues that AI memory should be reframed as a semiotic problem. A useful memory is not merely a stored text span; it is a structured relation between a sign, the object to which the sign refers, and an interpretant that renders the relation actionable. Once memory is understood in this way, phenomena that are peripheral in standard retrieval systems become central research questions: how signs drift across summaries, how competing interpretations are selected or rejected, how local experiences are promoted into group norms, and how multiple agents preserve object-level coherence while operating in partially different symbolic regimes [5], [6].

This reframing also reorganizes the role of adjacent disciplines. In the framework developed here, semiotics is the conceptual center; cognitive psychology explains how memory should be layered internally [1]–[4]; game theory explains why memory update is competitive rather than neutral; social diffusion explains how repeated exposure and clustered reinforcement turn local claims into shared conventions [7]; and computational linguistics provides measurable indicators of semantic drift, alias explosion, and interpretive divergence over time [8]. Together, these disciplines transform memory from a passive repository into an evolving order of meaning.

### 1.1 Contributions

The main contributions of this paper are as follows:

1. It reframes AI memory as a semiotic problem centered on sign-object-interpretant stability rather than as an enlarged retrieval buffer.
2. It reorganizes eight chapters of a broader research program into one coherent architecture spanning semiotic memory units, cognitive layers, self-play evolution, collective semiospheres, and propagation-aware auditing.
3. It positions FlashMemory as an object-aware research substrate for memory evolution rather than as a monolithic end-user agent platform.
4. It derives an evaluation agenda centered on semiotic stability, interpretive divergence, symbol grounding strength, canon quality, and repair efficiency.

### 1.2 Paper Organization

The rest of the paper follows a standard preprint arc. Section 2 reviews the most relevant work and clarifies the conceptual gap that motivates a semiotic reframing. Section 3 formalizes the problem statement and derives design requirements for evolving memory systems. Section 4 synthesizes the eight chapters into a unified semiotic memory architecture. Section 5 discusses FlashMemory as the substrate that makes the architecture empirically tractable. Section 6 presents an evaluation agenda and research roadmap. Section 7 discusses limitations and open questions, and Section 8 concludes.

## 2. Preliminaries and Related Work

### 2.1 Memory in Agent Systems

Recent work on agent memory has already shown that memory is more than short-term retrieval, but the field still lacks a unified account of meaning stability. Generative Agents introduced an architecture in which agents store experiences, retrieve relevant traces, and synthesize higher-level reflections that shape future behavior [9]. Reflexion showed that language-based self-feedback can function as a lightweight learning loop through an episodic memory buffer [10]. CAMEL demonstrated that multi-agent role-play exposes emergent cooperation patterns and differentiated communicative regimes [11]. MemGPT proposed a hierarchical memory scheme inspired by operating systems, making long contexts manageable through tiered paging [12]. Collectively, these systems establish that memory, reflection, and interaction matter. However, they still tend to treat memory primarily as stored text or context management rather than as a semiotic structure.

### 2.2 Cognitive Psychology and Internal Layering

Classical cognitive psychology offers a more principled account of internal memory organization. Atkinson and Shiffrin’s control-process model decomposes memory stores and routing functions [1]. Baddeley and Hitch show that working memory is not a passive buffer but an active workspace for manipulation [2]. Tulving’s distinction between episodic and semantic memory remains crucial: a system should differentiate between remembered events and stabilized knowledge claims [3]. Nelson and Narens add a meta-level perspective in which a system monitors its own uncertainty, freshness, and reliability [4]. These traditions suggest that a viable agent memory architecture must support stratified storage, controlled consolidation, and explicit self-monitoring.

### 2.3 Semiotics and Collective Meaning

Semiotics provides the conceptual center that current agent memory literature lacks. Peirce’s sign-object-interpretant triad makes explicit that meaning is not reducible to symbol and referent alone; interpretation is constitutive rather than secondary [5]. Lotman’s semiosphere extends this insight from individual interpretation to collective symbolic organization, emphasizing boundary, translation, core-periphery structure, and heterogeneous co-existence [6], [14]. This perspective is decisive for AI memory because multi-agent systems do not share one homogeneous language. Coding agents, reviewers, architects, documents, diagrams, and human operators each inhabit partially distinct symbolic regions. Many so-called memory failures are therefore better understood as failures of translation, alignment, or canonization rather than failures of storage.

### 2.4 Social Diffusion and Computational Linguistics

Two additional fields make the semiotic reframing operational. Social diffusion theory explains how repeated and clustered transmission converts local claims into shared conventions. Centola’s work is especially relevant because it shows that adoption often depends on reinforcement through network structure rather than on a single successful exposure [7]. Computational linguistics provides complementary tools for measuring how meaning changes over time. Hamilton et al. show that semantic drift can be studied quantitatively through diachronic representation change [8]. Together, these lines of work suggest that the right successor to current memory buffers is not simply a larger context window, but a propagation-aware architecture that tracks how signs move, compress, and drift across time and across agents.

## 3. Problem Statement and Design Requirements

The eight-chapter research package synthesized here starts from a stronger problem statement than most existing memory work. The central problem is not how to store more observations, but how to preserve operational meaning under repeated summarization, translation, contestation, and institutional reuse. A memory system fails when a sign remains available but no longer points to the same object, no longer supports a compatible interpretant, or has silently become detached from its original provenance. In long-horizon agent systems, these failures accumulate into brittle plans, sloganized norms, and collective misalignment.

This problem statement yields five design requirements. First, **object anchoring** must be explicit: every durable claim should be tied to inspectable objects such as code entities, documents, incidents, or decisions. Second, **interpretive plurality** must be representable: competing explanations should coexist as first-class objects rather than be collapsed into one overwritten summary. Third, **temporal provenance** must be preserved so that the system can trace when, where, and why a claim entered memory. Fourth, **staged consolidation** must separate transient observations from stabilized knowledge and institutional canon. Fifth, **contestability and repair** must be built into the system so that memory quality is judged through use, challenge, and revision rather than by write-time heuristics alone.

These requirements also clarify scope. The proposed framework is not a general theory of human meaning, nor a claim that every agent pipeline must explicitly implement every semiotic concept. Instead, it is a research program for engineering memory systems that remain interpretable, auditable, and updatable under evolution. The practical aim is to build memory architectures that can survive long horizons, heterogeneous roles, and repeated retellings without losing their object-level grounding.

## 4. A Semiotic Memory Architecture

### 4.1 Memory as a Semiotic Order

The first chapter of the underlying research program establishes the thesis that memory should be treated as an evolving semiotic order rather than an information store. This shift changes the design target from maximizing recall to maintaining operational meaning across time, task, and communication context. Once meaning stability becomes central, a memory architecture must explicitly model how claims are produced, circulated, contested, and stabilized. Object anchoring, provenance, and controlled promotion are no longer optional implementation details; they become constitutive parts of the memory design.

The practical consequence is that memory can no longer be modeled as a flat cache. Durable memory must reserve space for ambiguity, conflicting interpretations, and revision histories. A system that stores only paraphrased text will gradually accumulate self-referential summaries and unstable slogans. A system that stores sign-object-interpretant relations, in contrast, can distinguish between lexical variation and genuine semantic change. This thesis provides the theoretical hinge for the remaining architectural components.

### 4.2 Semiotic Memory Units

The second chapter contributes the minimal research object for the architecture: the **semiotic memory unit (SMU)**. An SMU is not a generic chunk, note, or message. It is a claim-level structure containing at least seven fields: `sign`, `object`, `interpretant`, `context`, `provenance`, `confidence`, and `social_status`. This design operationalizes Peirce’s triad and extends it with the minimum controls needed for engineering and experimentation. The `sign` captures surface expression, the `object` anchors the claim to inspectable entities, and the `interpretant` records the actionable understanding. `Context` localizes use conditions, `provenance` makes the claim auditable, `confidence` calibrates trust, and `social_status` indicates whether the claim is a private cue, local hypothesis, contested interpretation, provisional consensus, or institutional canon.

The key advantage of the SMU abstraction is that it separates object identity from interpretive plurality. Competing explanations are stored as distinct units connected by relations such as `supports`, `contests`, or `supersedes`, rather than being merged into one fuzzy summary. This makes interpretation a manipulable research object instead of an accidental side effect of prompt phrasing. It also gives the architecture an operational atom that can be indexed, promoted, challenged, deprecated, and measured over time.

> [Figure 2 Placeholder]
> SMU schema figure: a central `Semiotic Memory Unit` node connected to `sign / object / interpretant / context / provenance / confidence / social_status / relations / metrics / timestamps`.
> Use `FIG-2` in [2026-04-23-semiotics-centered-memory-survey-paper-author-notes.md](./2026-04-23-semiotics-centered-memory-survey-paper-author-notes.md).

### 4.3 Cognitive Layers and Memory Physiology

The third chapter imports cognitive psychology to specify how SMUs should be processed inside an agent. The proposal is a five-layer physiology consisting of perceptual buffer, working memory, episodic memory, semantic memory, and metamemory. This layering matters because the same unit should not play every role simultaneously. Raw observations belong in a transient buffer; task-relevant interpretations belong in working memory; event traces belong in episodic memory; stabilized claims belong in semantic memory; and freshness, uncertainty, conflict, and retrieval diagnostics belong in metamemory [1]–[4].

This layered perspective guards against two recurring pathologies. The first is context sprawl, in which every observation is treated as equally durable and the system becomes noisy and brittle. The second is premature canonization, in which a single event is generalized too early and becomes an overconfident norm. By distinguishing where an SMU lives and how it moves, the architecture turns memory management into a principled theory of routing, consolidation, and forgetting.

### 4.4 Self-Play Harness as an Evolution Engine

The fourth chapter argues that memory quality cannot be determined at write time alone. A claim proves itself only when it is retrieved, acted on, challenged, and either reinforced or repaired. The `self-play harness` is therefore defined not as a benchmark runner but as a memory evolution environment. It generates tasks, assigns heterogeneous roles, stages reflection or debate, judges downstream outcomes, and updates memory state accordingly. In this setting, self-play is not merely agent-versus-agent competition; it is a systematic mechanism for exposing unstable interpretations, overconfident abstractions, and brittle conventions [10], [11], [13], [15].

From a semiotic perspective, the harness is where interpretants compete. From a game-theoretic perspective, it is where claims acquire or lose strategic value under repeated use. The harness supplies the selection mechanism that a semiotic memory theory requires. Without it, memory remains archival. With it, memory becomes evolutionary, because the system can reward grounded, reusable interpretations and demote misleading or weakly anchored ones.

### 4.5 Semiosphere, Propagation, and Collective Memory

The fifth and sixth chapters scale the problem from individual agents to collective symbolic ecologies. Following Lotman, the architecture treats a multi-agent or human-agent environment as a **semiosphere** composed of partially overlapping symbolic regions [6], [14]. Code review discourse, incident discourse, product discourse, architectural discourse, and tacit human discourse are not interchangeable channels; they are structured regions with different translation costs and different risks of distortion. Translation across these regions is both a source of innovation and a source of error.

Propagation further complicates the picture. Memory does not simply persist; it is retold, compressed, translated, and redistributed. During this process, signs may drift away from objects, aliases may proliferate, and local explanations may harden into misleading slogans. Computational linguistics makes this process measurable through drift signals, anchor retention, and alias growth [8]. Social diffusion theory explains why some claims become norms only after repeated and clustered reinforcement [7]. Collective memory is therefore not reducible to many individual memories in parallel; it is a structured symbolic field in motion.

> [Figure 3 Placeholder]
> Collective memory and semiosphere figure: multiple semantic regions (`Code`, `Design`, `Incident`, `Product`, `Human Tacit`) surrounding a shared object graph, connected by boundary filters and translation edges; the right side shows propagation, compression, drift, and stabilization.
> Use `FIG-3` in [2026-04-23-semiotics-centered-memory-survey-paper-author-notes.md](./2026-04-23-semiotics-centered-memory-survey-paper-author-notes.md).

### 4.6 FlashMemory as the Substrate Layer

The seventh chapter grounds the architecture in FlashMemory. Rather than positioning FlashMemory as a fully fledged agent product, the chapter interprets it as the substrate responsible for object anchoring, multimodal sign indexing, temporal provenance, graph relations, and research traceability. This is a strategic narrowing. A semiotic architecture needs a stable object layer more than it needs another monolithic orchestration stack. FlashMemory is promising because it already sits close to inspectable objects: code entities, documents, graph relations, and cross-artifact links.

This substrate view also clarifies division of labor. FlashMemory should provide object graph management, multimodal sign indexing, temporal provenance, and retrieval/assembly primitives for working memory. The harness should consume those primitives to run trials, debates, conflict-resolution episodes, and policy updates. Agents should inhabit the system as role-bearing interpreters rather than as the owners of memory itself. This separation makes the full program more modular, more auditable, and more scientifically tractable.

## 5. FlashMemory as a Semiotic Research Substrate

The architecture above implies a strategic claim about infrastructure. If semiotic memory requires object anchoring, provenance, temporal traceability, and multimodal symbolic alignment, then the substrate must be optimized for those functions before any end-to-end memory product is judged. FlashMemory is especially valuable because it allows researchers to bind symbolic claims to inspectable artifacts. This makes it possible to replay memory evolution, inspect contested claims, trace canonization events, and compare propagation regimes across time rather than relying on anecdotal agent transcripts.

This substrate framing also matters methodologically. A large share of current agent-memory research is difficult to reproduce because memory is hidden inside prompts, opaque summaries, or ad hoc JSON blobs. By contrast, a FlashMemory-centered substrate can expose object anchors, temporal lineage, relation graphs, and promotion histories as inspectable research objects. In that sense, the substrate is not merely infrastructure. It is part of the epistemic method required to turn semiotic memory from an attractive metaphor into a falsifiable systems program.

## 6. Evaluation Agenda and Research Roadmap

For the eight-chapter program to become an empirical research agenda, the evaluation story must be as clear as the architectural story. The first family of metrics concerns **semiotic stability**: after a claim is summarized, translated, debated, or propagated across agents, does it still point to the same object and sustain a compatible interpretant? The second concerns **interpretive divergence**: when multiple agents face the same object, how much do their interpretants disagree, and is the disagreement explicit or latent? The third is **symbol grounding strength**: are the signs in a claim still anchored to inspectable objects, or have they drifted into free-floating slogans?

The next family concerns norm formation and repair. **Canon quality** asks whether the claims promoted into shared norms are in fact reusable, precise, and low-risk. **Repair efficiency** asks how quickly the system recovers once a weak interpretation is established. These metrics matter because the most expensive failures in long-lived memory systems may come not from isolated retrieval misses, but from bad norms that are repeatedly and confidently reused. Evaluating evolving memory therefore requires institutional metrics in addition to individual correctness metrics.

From an experimental perspective, the framework yields a natural comparison matrix. One axis varies memory representation: plain chunks, object-anchored chunks, or full SMUs. A second varies internal layering: flat memory versus layered memory. A third varies evolution mechanism: no reflection, single-agent reflection, debate, or full self-play harness. A fourth varies social structure: single agent, human-agent pair, multi-agent cluster, or multi-region semiosphere. This design keeps the program falsifiable. If semiotic memory does not help, it should fail under controlled comparison. If it does help, it should do so through measurable gains in stability, grounding, and repair, not through anecdotal impressions alone.

> [Figure 4 Placeholder]
> Evaluation roadmap figure: a four-dimensional experiment matrix (`Memory Unit / Memory Layers / Evolution Mechanism / Social Structure`) linked to five metric families (`Semiotic Stability`, `Interpretive Divergence`, `Symbol Grounding Strength`, `Canon Quality`, `Repair Efficiency`).
> Use `FIG-4` in [2026-04-23-semiotics-centered-memory-survey-paper-author-notes.md](./2026-04-23-semiotics-centered-memory-survey-paper-author-notes.md).

## 7. Discussion, Limitations, and Open Questions

The strongest claim of this paper is also the easiest to misunderstand. Placing semiotics at the center does not reduce AI systems to a humanities metaphor. It highlights that many advanced memory failures are fundamentally failures of sign-object-interpretant alignment. Cognitive psychology alone cannot explain collective norm formation. Game theory alone cannot explain why two agents appear to disagree while actually referring to different objects. Social diffusion alone cannot explain why repeated claims sometimes stabilize as empty slogans. Computational linguistics can track drift, but cannot by itself define what should count as stable meaning. Semiotics provides the integrative layer within which these disciplines become mutually constraining rather than loosely adjacent.

The proposal also has clear limitations. First, semiotic representations increase schema and annotation overhead. Second, object anchoring is natural in software repositories but harder in open-world domains where inspectable objects are less sharply defined. Third, social-status transitions require governance rules that avoid both premature canonization and excessive conservatism. Fourth, the field still lacks large-scale benchmarks tailored to contested meanings and cross-semiosphere translation. These are not incidental implementation details; they define the real scientific workload ahead.

Several open questions follow directly. How should promotion thresholds vary across domains with different costs of false canonization? What kinds of harness-induced conflict are most diagnostic of weak interpretants? How should semiospheric boundaries be learned rather than manually specified? Which representations best expose the difference between lexical variation and object-level drift? These questions suggest that the proposed architecture is not the endpoint of a design exercise, but the beginning of a measurable research program.

## 8. Conclusion

This paper has reorganized eight chapters of a larger research package into a single arXiv-style survey-position manuscript. The main conclusion is that evolving AI memory should be built around semiotic structure rather than treated as expanded context management. The resulting architecture begins with semiotic memory units, scales through cognitive layers and self-play harnesses, expands into collective semiospheres and propagation-aware auditing, and is grounded by FlashMemory as an object-aware substrate. Cognitive psychology explains internal layering, game theory explains competition and update, social diffusion explains norm formation, and computational linguistics explains how drift becomes measurable. Semiotics remains the center because it alone explains why preserving meaning, rather than merely preserving content, is the defining long-horizon challenge.

If this synthesis is correct, the next generation of AI memory research will no longer ask only how to store more, summarize better, or retrieve faster. It will ask how to preserve and govern meaning across agents, artifacts, timescales, and institutions. That is a harder question, but it is also the question most worth answering.

## References

[1] R. C. Atkinson and R. M. Shiffrin, “Human Memory: A Proposed System and its Control Processes,” in *Psychology of Learning and Motivation*, 1968. [Link](https://doi.org/10.1016/S0079-7421(08)60422-3)

[2] A. D. Baddeley and G. J. Hitch, “Working Memory,” in *Psychology of Learning and Motivation*, 1974.

[3] E. Tulving, “Episodic and Semantic Memory,” in *Organization of Memory*, 1972. [Link](https://cir.nii.ac.jp/crid/1574231874408386176?lang=en)

[4] T. O. Nelson and L. Narens, “Metamemory: A Theoretical Framework and New Findings,” in *Psychology of Learning and Motivation*, 1990. [Link](https://doi.org/10.1016/S0079-7421(08)60053-5)

[5] T. L. Short, “Peirce’s Theory of Signs,” *Stanford Encyclopedia of Philosophy*, 2021 archive. [Link](https://plato.stanford.edu/archives/sum2021/entries/peirce-semiotics/)

[6] J. Lotman and W. Clark, “On the Semiosphere,” *Sign Systems Studies*, vol. 33, no. 1, pp. 205–229, 2005. [Link](https://doi.org/10.12697/SSS.2005.33.1.09)

[7] D. Centola, “The Spread of Behavior in an Online Social Network Experiment,” *Science*, vol. 329, no. 5996, pp. 1194–1197, 2010. [Link](https://doi.org/10.1126/science.1185231)

[8] W. L. Hamilton, J. Leskovec, and D. Jurafsky, “Diachronic Word Embeddings Reveal Statistical Laws of Semantic Change,” in *ACL 2016*, pp. 1489–1501, 2016. [Link](https://aclanthology.org/P16-1141/)

[9] J. S. Park, J. C. O’Brien, C. J. Cai, M. R. Morris, P. Liang, and M. S. Bernstein, “Generative Agents: Interactive Simulacra of Human Behavior,” in *UIST 2023*, 2023. [Link](https://arxiv.org/abs/2304.03442)

[10] N. Shinn, F. Cassano, A. Gopinath, K. Narasimhan, and S. Yao, “Reflexion: Language Agents with Verbal Reinforcement Learning,” in *NeurIPS 2023*, 2023. [Link](https://proceedings.neurips.cc/paper_files/paper/2023/hash/1b44b878bb782e6954cd888628510e90-Abstract-Conference.html)

[11] G. Li, H. A. A. K. Hammoud, H. Itani, D. Khizbullin, and B. Ghanem, “CAMEL: Communicative Agents for ‘Mind’ Exploration of Large Language Model Society,” *NeurIPS 2023*. [Link](https://arxiv.org/abs/2303.17760)

[12] C. Packer, S. Wooders, K. Lin, V. Fang, S. G. Patil, I. Stoica, and J. E. Gonzalez, “MemGPT: Towards LLMs as Operating Systems,” 2024. [Link](https://arxiv.org/abs/2310.08560)

[13] A. Madaan et al., “Self-Refine: Iterative Refinement with Self-Feedback,” 2023. [Link](https://doi.org/10.48550/arXiv.2303.17651)

[14] W. Nöth, “The Topography of Yuri Lotman’s Semiosphere,” *International Journal of Cultural Studies*, vol. 18, no. 1, pp. 11–26, 2015. [Link](https://doi.org/10.1177/1367877914528114)

[15] S. Yao et al., “ReAct: Synergizing Reasoning and Acting in Language Models,” *ICLR 2023*. [Link](https://arxiv.org/abs/2210.03629)

> Author drafting notes, figure prompts, and internal self-review are maintained separately in [2026-04-23-semiotics-centered-memory-survey-paper-author-notes.md](./2026-04-23-semiotics-centered-memory-survey-paper-author-notes.md).
