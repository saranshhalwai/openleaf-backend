package projects

import "strings"

// TemplateFile represents a single file contained within a LaTeX starter template
type TemplateFile struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

// TemplateInfo provides metadata about a template for listing in the UI
type TemplateInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Category    string `json:"category"`
	Icon        string `json:"icon"`
}

// Template represents a full template with all associated starter files
type Template struct {
	TemplateInfo
	RootFile string         `json:"rootFile"`
	Files    []TemplateFile `json:"files"`
}

var defaultArticleTeX = `\documentclass{article}
\usepackage[utf8]{inputenc}
\usepackage{amsmath, amssymb}
\usepackage{graphicx}
\usepackage{hyperref}

\title{My Openleaf Document}
\author{Openleaf Collaborator}
\date{\today}

\begin{document}

\maketitle

\begin{abstract}
This is a concise abstract describing the objectives, methodology, and key contributions of this document.
\end{abstract}

\section{Introduction}
Welcome to your new Openleaf \LaTeX{} project! You can collaborate in real-time with colleagues, edit source code with rich syntax assistance, and compile to PDF with TeX Live.

\section{Mathematics}
\LaTeX{} makes mathematical typesetting intuitive. For instance, Euler's formula states:
\begin{equation}
e^{i\pi} + 1 = 0
\end{equation}

And the Gaussian integral is given by:
\begin{equation}
\int_{-\infty}^{\infty} e^{-x^2} \, dx = \sqrt{\pi}
\end{equation}

\section{Conclusion}
Start writing your document here! Add more sections, figures, and tables as needed.

\end{document}
`

var ieeeMainTeX = `\documentclass[conference]{IEEEtran}
\usepackage[utf8]{inputenc}
\usepackage{cite}
\usepackage{amsmath,amssymb,amsfonts}
\usepackage{algorithmic}
\usepackage{graphicx}
\usepackage{textcomp}
\usepackage{xcolor}

\begin{document}

\title{Title of IEEE Conference Paper*\\
{\footnotesize \textsuperscript{*}Note: Sub-titles are not recommended for conference papers.}}

\author{\IEEEauthorblockN{First Author}
\IEEEauthorblockA{\textit{Department of Computer Science} \\
\textit{University / Organization}\\
City, Country \\
author1@example.com}
\and
\IEEEauthorblockN{Second Author}
\IEEEauthorblockA{\textit{Department of Electrical Engineering} \\
\textit{University / Organization}\\
City, Country \\
author2@example.com}
}

\maketitle

\begin{abstract}
This electronic document is a live template. The abstract should briefly state the purpose of the research, the principal results and major conclusions. An abstract should typically not exceed 200 words.
\end{abstract}

\begin{IEEEkeywords}
collaborative editing, conflict-free replicated data types, LaTeX compilation, cloud systems
\end{IEEEkeywords}

\section{Introduction}
This document is a conference template for \LaTeX{}. Real-time collaborative document preparation has become a cornerstone of modern scientific communication~\cite{lamport1994latex}.

\section{Related Work}
Prior work demonstrates the efficacy of real-time collaborative editing algorithms and distributed synchronization~\cite{shannon1948mathematical}.

\section{Methodology}
Equations must be numbered sequentially:
\begin{equation}
\alpha + \beta = \gamma
\label{eq:gamma}
\end{equation}
Refer to \eqref{eq:gamma} within the body of the paper.

\begin{table}[htbp]
\caption{System Performance Metrics}
\begin{center}
\begin{tabular}{|c|c|c|}
\hline
\textbf{Metric} & \textbf{Baseline} & \textbf{Openleaf} \\
\hline
Compile Time (s) & 8.4 & 2.1 \\
Sync Latency (ms) & 120 & 18 \\
\hline
\end{tabular}
\label{tab:metrics}
\end{center}
\end{table}

\section{Conclusion}
We have presented an IEEE conference paper template for Openleaf.

\bibliographystyle{IEEEtran}
\bibliography{references}

\end{document}
`

var ieeeBibTeX = `@book{lamport1994latex,
  title={LaTeX: A Document Preparation System},
  author={Lamport, Leslie},
  year={1994},
  publisher={Addison-Wesley}
}

@article{shannon1948mathematical,
  title={A mathematical theory of communication},
  author={Shannon, Claude E},
  journal={The Bell System Technical Journal},
  volume={27},
  number={3},
  pages={379--423},
  year={1948},
  publisher={Nokia Bell Labs}
}
`

var resumeMainTeX = `\documentclass[11pt,a4paper]{article}
\usepackage[utf8]{inputenc}
\usepackage[margin=0.75in]{geometry}
\usepackage{hyperref}
\usepackage{enumitem}
\usepackage{titlesec}

% Section formatting
\titleformat{\section}{\large\bfseries\uppercase}{}{0em}{}[\titlerule]
\titlespacing{\section}{0pt}{10pt}{6pt}

\pagestyle{empty}

\begin{document}

% Header
\begin{center}
    {\Huge \textbf{Jane Doe}}\\[4pt]
    \href{mailto:jane.doe@example.com}{jane.doe@example.com} $\vert$ +1 (555) 123-4567 $\vert$ \href{https://github.com}{github.com/janedoe} $\vert$ San Francisco, CA
\end{center}

\vspace{6pt}

% Education
\section{Education}
\textbf{University of California, Berkeley} \hfill Berkeley, CA\\
\textit{B.S. in Computer Science} \hfill \textit{Aug 2020 -- May 2024}\\
GPA: 3.9/4.0 $\cdot$ Dean's Honors List

\vspace{4pt}

% Experience
\section{Experience}
\textbf{Software Engineering Intern} $\vert$ \textit{Cloud Solutions Inc.} \hfill \textit{May 2023 -- Aug 2023}
\begin{itemize}[leftmargin=1.5em, itemsep=1pt]
    \item Designed and deployed distributed microservices in Go, reducing latency by 35\%.
    \item Implemented real-time WebSocket communication channels handling 50k+ daily events.
    \item Collaborated with frontend engineers to integrate reactive UI components.
\end{itemize}

\textbf{Undergraduate Research Assistant} $\vert$ \textit{Systems Lab} \hfill \textit{Jan 2022 -- Dec 2022}
\begin{itemize}[leftmargin=1.5em, itemsep=1pt]
    \item Investigated distributed consensus algorithms and conflict-free replicated data types.
    \item Authored technical reports and experimental benchmarking scripts in Python.
\end{itemize}

\vspace{4pt}

% Projects
\section{Projects}
\textbf{Openleaf Collaborative Editor} $\vert$ \textit{Go, SvelteKit, Yjs, Docker, GCP}
\begin{itemize}[leftmargin=1.5em, itemsep=1pt]
    \item Built high-performance collaborative LaTeX editor supporting up to 6 real-time peers per document.
    \item Engineered sandboxed asynchronous compilation pipeline producing PDF previews in under 3 seconds.
\end{itemize}

\vspace{4pt}

% Skills
\section{Technical Skills}
\textbf{Languages:} Go, TypeScript, Python, C++, SQL, \LaTeX\\
\textbf{Frameworks \& Tools:} SvelteKit, Docker, Kubernetes, Google Cloud, Git, PostgreSQL

\end{document}
`

var beamerMainTeX = `\documentclass{beamer}
\usepackage[utf8]{inputenc}

\usetheme{Madrid}
\usecolortheme{beaver}

\title[Openleaf Presentation]{Collaborative \LaTeX{} Presentations}
\subtitle{Built with Modern Openleaf Engine}
\author{Alex Morgan}
\institute{Department of Research}
\date{\today}

\begin{document}

% Title slide
\begin{frame}
    \titlepage
\end{frame}

% Outline slide
\begin{frame}{Outline}
    \tableofcontents
\end{frame}

\section{Introduction}

\begin{frame}{Why \LaTeX{} for Slides?}
    \begin{itemize}
        \item \textbf{Mathematical precision}: Beautiful equations with zero layout drift.
        \item \textbf{Consistency}: Structured themes and styling across every slide.
        \item \textbf{Plaintext portability}: Track revisions cleanly with Git and CRDT algorithms.
    \end{itemize}
\end{frame}

\section{Core Capabilities}

\begin{frame}{Key Features}
    \begin{block}{Real-Time Synchronization}
        Synchronize slide edits instantaneously with your co-authors via binary CRDT relays.
    \end{block}

    \begin{alertblock}{Cloud Compiles}
        Asynchronously compile directly to presentation-ready PDF on Google Cloud Run.
    \end{alertblock}
\end{frame}

\section{Conclusion}

\begin{frame}{Summary \& Next Steps}
    \begin{enumerate}
        \item Customize this template with your slides.
        \item Add figures, tables, code listings, and equations.
        \item Compile and present in full-screen mode!
    \end{enumerate}
    
    \vspace{12pt}
    \centering
    \textbf{Thank you! Any Questions?}
\end{frame}

\end{document}
`

var availableTemplates = []Template{
	{
		TemplateInfo: TemplateInfo{
			ID:          "article",
			Name:        "Standard Article",
			Description: "Clean single-column article with title, abstract, math equations, and sections.",
			Category:    "Academic",
			Icon:        "📄",
		},
		RootFile: "main.tex",
		Files: []TemplateFile{
			{Path: "main.tex", Content: defaultArticleTeX},
		},
	},
	{
		TemplateInfo: TemplateInfo{
			ID:          "ieee",
			Name:        "IEEE Conference Paper",
			Description: "Two-column conference paper format with abstract, keywords, table, and bibliography.",
			Category:    "Academic",
			Icon:        "📜",
		},
		RootFile: "main.tex",
		Files: []TemplateFile{
			{Path: "main.tex", Content: ieeeMainTeX},
			{Path: "references.bib", Content: ieeeBibTeX},
		},
	},
	{
		TemplateInfo: TemplateInfo{
			ID:          "resume",
			Name:        "Professional Resume / CV",
			Description: "Modern single-page resume layout with education, experience, projects, and skills.",
			Category:    "Career",
			Icon:        "💼",
		},
		RootFile: "main.tex",
		Files: []TemplateFile{
			{Path: "main.tex", Content: resumeMainTeX},
		},
	},
	{
		TemplateInfo: TemplateInfo{
			ID:          "beamer",
			Name:        "Presentation Slides (Beamer)",
			Description: "Slide presentation with Madrid theme, outline, blocks, and clean typography.",
			Category:    "Presentation",
			Icon:        "📽️",
		},
		RootFile: "main.tex",
		Files: []TemplateFile{
			{Path: "main.tex", Content: beamerMainTeX},
		},
	},
}

// ListTemplates returns metadata about all supported starter templates
func ListTemplates() []TemplateInfo {
	infos := make([]TemplateInfo, len(availableTemplates))
	for i, t := range availableTemplates {
		infos[i] = t.TemplateInfo
	}
	return infos
}

// GetTemplate resolves a template by ID, falling back to the standard article template
func GetTemplate(id string) Template {
	cleanID := strings.ToLower(strings.TrimSpace(id))
	for _, t := range availableTemplates {
		if strings.EqualFold(t.ID, cleanID) {
			return t
		}
	}
	// Fallback to standard article
	return availableTemplates[0]
}
