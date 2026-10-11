package web

const (
	// On pointe désormais sur le fichier local servi par /static/
	PicoCSSURL = "/static/pico.min.css"

	// CSP durcie : plus aucun domaine tiers externe n'est requis pour les styles
	PicoCSP = "default-src 'none'; img-src 'self' data: blob:; media-src 'self' blob:; style-src 'self' 'unsafe-inline'; script-src 'self' 'wasm-unsafe-eval'; connect-src 'self' https:; form-action 'self'; base-uri 'none'; frame-ancestors 'none'"

	PicoThemeCSS = `
		:root {
			--page-width: 44rem;
		}

		/* Masquage de l'affichage de la barre de défilement vertical */
		html,
		body,
		* {
			scrollbar-width: none;
			-ms-overflow-style: none;
		}

		html::-webkit-scrollbar,
		body::-webkit-scrollbar,
		*::-webkit-scrollbar {
			display: none;
			width: 0;
			height: 0;
		}

		html,
		body {
			overflow-x: hidden;
			max-width: 100%;
			width: 100%;
		}

		main.container {
			max-width: var(--page-width);
			padding-top: 8vh;
			padding-bottom: 8vh;
			overflow-x: hidden;
		}
		header { margin-bottom: 2rem; }
		header h1 { margin-bottom: .4rem; }
		.muted { color: var(--pico-muted-color); }

		/* Logo Blossom / PNG */
		.logo-box {
			display: flex;
			justify-content: center;
			align-items: center;
			width: 100%;
			margin-bottom: 2rem;
			background: transparent !important;
			background-color: transparent !important;
			border: none !important;
			outline: none !important;
			box-shadow: none !important;
			overflow: hidden !important;
		}

		.logo-link {
			display: block;
			text-decoration: none !important;
			color: inherit !important;
			border: none !important;
			outline: none !important;
			width: 100%;
			max-width: var(--page-width);
			text-align: center;
			background: transparent !important;
			background-color: transparent !important;
			-webkit-touch-callout: none !important;
			-webkit-user-select: none !important;
			-khtml-user-select: none !important;
			-moz-user-select: none !important;
			-ms-user-select: none !important;
			user-select: none !important;
			-webkit-user-drag: none !important;
		}

		.logo-link:hover,
		.logo-link:focus,
		.logo-link:active {
			text-decoration: none !important;
			color: inherit !important;
			outline: none !important;
			border: none !important;
			box-shadow: none !important;
			background: transparent !important;
			background-color: transparent !important;
		}

		.site-logo,
		.svg-logo {
			width: 100%;
			max-width: var(--page-width);
			height: auto;
			display: block;
			margin: 0 auto;
			border: none !important;
			outline: none !important;
			box-shadow: none !important;
			background: transparent !important;
			background-color: transparent !important;
			-webkit-touch-callout: none !important;
			-webkit-user-select: none !important;
			-khtml-user-select: none !important;
			-moz-user-select: none !important;
			-ms-user-select: none !important;
			user-select: none !important;
			-webkit-user-drag: none !important;
			user-drag: none;
			pointer-events: auto;
		}

		.site-logo:hover,
		.site-logo:focus,
		.site-logo:active,
		.svg-logo:hover,
		.svg-logo:focus,
		.svg-logo:active {
			outline: none !important;
			border: none !important;
			box-shadow: none !important;
		}

		.site-logo::selection,
		.svg-logo::selection,
		.logo-link::selection {
			background: transparent !important;
		}

		.share-box {
			display: flex;
			flex-direction: row;
			align-items: stretch;
			gap: 1.5rem;
		}

		@media (max-width: 600px) {
			.share-box {
				flex-direction: column;
				align-items: center;
			}
			.share-qr {
				width: min(240px, 100%);
				height: auto;
			}
		}

		.share-qr {
			display: flex;
			align-items: center;
			justify-content: center;
			margin: 0;
			flex-shrink: 0;
			align-self: stretch;
			aspect-ratio: 1 / 1;
			background-color: #181C25;
			border-radius: var(--pico-border-radius);
		}

		.share-qr img {
			display: block;
			width: 100%;
			height: auto;
			border: none;
			border-radius: calc(var(--pico-border-radius) / 2);
		}

		.success {
			border: 1px solid var(--pico-primary);
		}

		.share-details {
			display: flex;
			flex-direction: column;
			gap: 0.75rem;
			min-width: 0;
			flex: 1;
		}

		.share-details .meta-item {
			display: flex;
			flex-direction: column;
			gap: 0.15rem;
		}

		.share-details .meta-item strong {
			overflow-wrap: anywhere;
		}

		.share-details .meta-item code {
			display: block;
			font-size: 0.72rem;
			line-height: 1.35;
			padding: 0.35rem 0.5rem;
			word-break: break-all;
			overflow-wrap: anywhere;
			border: 1px solid var(--pico-muted-border-color);
			border-radius: var(--pico-border-radius);
		}

		/* Boutons d'action No-JS avec indicateur d'état interactif */
		@keyframes btn-spin {
			0% { transform: rotate(0deg); }
			100% { transform: rotate(360deg); }
		}

		#uploadForm {
			position: relative;
		}

		#uploadForm label,
		#uploadForm input {
			position: relative;
			z-index: 2;
		}

		.btn-action {
			position: relative;
			display: inline-flex;
			align-items: center;
			justify-content: center;
			text-align: center;
			transition: background-color var(--pico-transition), border-color var(--pico-transition), opacity var(--pico-transition);
		}

		.btn-action .btn-text,
		.btn-action .btn-loading {
			position: relative;
			z-index: 2;
		}

		.upload-btn-mascot {
			position: absolute;
			bottom: 100%;
			right: 3rem;
			height: 205px;
			width: auto;
			max-width: 100%;
			pointer-events: none;
			user-select: none !important;
			-webkit-user-select: none !important;
			-moz-user-select: none !important;
			-ms-user-select: none !important;
			-webkit-user-drag: none !important;
			user-drag: none;
			z-index: 1;
			opacity: 0.5;
			transition: opacity var(--pico-transition);
		}

		.btn-action:hover .upload-btn-mascot,
		.btn-action:focus .upload-btn-mascot {
			opacity: 0.75;
		}

		@media (max-width: 768px) {
			.upload-btn-mascot {
				height: 130px;
				right: 0;
				opacity: 0.2;
				z-index: 0;
				max-width: 50%;
			}
			.btn-action:hover .upload-btn-mascot,
			.btn-action:focus .upload-btn-mascot {
				opacity: 0.2;
			}
		}

		@media (max-width: 480px) {
			.upload-btn-mascot {
				height: 100px;
				right: 0;
				opacity: 0.18;
				z-index: 0;
				max-width: 45%;
			}
		}

		/* Mascotte du champ captcha : même base mais opacité fixe en responsive,
		   s'éclaircit quand on clique dans l'input. */
		.captcha-field .upload-btn-mascot {
			opacity: 0.5;
		}

		.captcha-field:focus-within .upload-btn-mascot {
			opacity: 0.75;
		}

		@media (max-width: 768px) {
			.captcha-field .upload-btn-mascot {
				opacity: 0.5;
			}

			.captcha-field:focus-within .upload-btn-mascot {
				opacity: 0.75;
			}
		}

		@media (max-width: 480px) {
			.captcha-field .upload-btn-mascot {
				opacity: 0.5;
			}

			.captcha-field:focus-within .upload-btn-mascot {
				opacity: 0.75;
			}
		}




		.btn-action .btn-text {
			transition: opacity 0s linear 999s, visibility 0s linear 999s;
		}

		.btn-action:disabled {
			opacity: 1 !important;
			color: #fff !important;
		}

		.btn-action .btn-loading {
			display: inline-flex;
			align-items: center;
			justify-content: center;
			gap: 0.45rem;
			position: absolute;
			inset: 0;
			max-width: 100%;
			overflow: hidden;
			font-size: inherit;
			line-height: inherit;
			opacity: 0;
			visibility: hidden;
			transition: opacity 0s linear 999s, visibility 0s linear 999s;
			pointer-events: none;
		}

		/* Startup-disabled is not loading: the JS upload handler controls this state. */
		#submitBtn .btn-text,
		#submitBtn .btn-loading {
			transition: none;
		}

		#submitBtn.is-loading .btn-text {
			visibility: hidden !important;
			opacity: 0 !important;
		}

		#submitBtn.is-loading .btn-loading {
			visibility: visible !important;
			opacity: 1 !important;
			color: #fff !important;
		}

		.btn-spinner {
			display: inline-block;
			width: 0.9em;
			height: 0.9em;
			border: 2px solid currentColor;
			border-right-color: transparent;
			border-radius: 50%;
			animation: btn-spin 0.75s linear infinite;
			vertical-align: middle;
			flex-shrink: 0;
			margin-right: 8px;
		}

		/* Liens de téléchargement et déchiffrement : persistant après blur via transition-delay */
		a.btn-action:active .btn-text,
		a.btn-action:focus .btn-text {
			visibility: hidden !important;
			opacity: 0 !important;
			transition-delay: 0s, 0s !important;
		}

		a.btn-action:active .btn-loading,
		a.btn-action:focus .btn-loading {
			visibility: visible !important;
			opacity: 1 !important;
			transition-delay: 0s, 0s !important;
		}

		/* No-JS upload displays progress only during a valid button press. */
		#file:valid ~ #submitBtn:active .btn-text,
		form:has(#file:valid) #submitBtn:active .btn-text {
			visibility: hidden !important;
			opacity: 0 !important;
		}

		#file:valid ~ #submitBtn:active .btn-loading,
		form:has(#file:valid) #submitBtn:active .btn-loading {
			visibility: visible !important;
			opacity: 1 !important;
		}

		/* Styles de la zone de prévisualisation */
		.preview-box {
			display: flex;
			justify-content: center;
			align-items: center;
			padding: 1rem;
			background-color: #181C25;
			border-radius: var(--pico-border-radius);
			margin-bottom: 1.5rem;
			overflow: hidden;
		}

		.preview-media {
			max-width: 100%;
			max-height: 70vh;
			border-radius: calc(var(--pico-border-radius) / 2);
			object-fit: contain;
			display: block;
		}

		.preview-audio {
			width: 100%;
		}

		.no-preview {
			padding: 2rem;
			text-align: center;
			width: 100%;
		}

		.no-preview-cross {
			display: block;
			width: 128px;
			height: 128px;
			max-width: 100%;
			margin: 1rem auto 0;
			object-fit: contain;
		}

		/* Animation et état de chargement Zero-Knowledge / RLNC */
		.zk-loading-container {
			display: flex;
			flex-direction: column;
			align-items: center;
			justify-content: center;
			padding: 3.5rem 1.5rem;
			text-align: center;
			gap: 1.75rem;
			min-height: 40vh;
		}

		.zk-loading-card {
			display: inline-flex;
			align-items: center;
			justify-content: center;
			padding: 1.5rem;
			background: var(--pico-card-background-color);
			border: 1px solid var(--pico-primary);
			border-radius: var(--pico-border-radius);
			box-shadow: var(--pico-card-box-shadow);
		}

		.zk-loading-gif {
			width: 192px;
			max-width: min(192px, 75vw);
			height: auto;
			display: block;
			object-fit: contain;
			filter:
				drop-shadow(0 0 1.5px rgba(255, 255, 255, 0.65))
				drop-shadow(0 0 12px rgba(248, 105, 191, 0.38))
				drop-shadow(0 8px 16px rgba(0, 0, 0, 0.45));
		}

		@media (prefers-color-scheme: light) {
			:root:not([data-theme="dark"]) .zk-loading-gif {
				filter: drop-shadow(0 0 2px rgba(0, 0, 0, 0.15)) drop-shadow(0 0 10px rgba(193, 32, 139, 0.25)) drop-shadow(0 8px 16px rgba(0, 0, 0, 0.12));
			}
		}

		[data-theme="light"] .zk-loading-gif {
			filter: drop-shadow(0 0 2px rgba(0, 0, 0, 0.15)) drop-shadow(0 0 10px rgba(193, 32, 139, 0.25)) drop-shadow(0 8px 16px rgba(0, 0, 0, 0.12));
		}

		.zk-spinner {
			width: 52px;
			height: 52px;
			border: 4px solid var(--pico-muted-border-color);
			border-top-color: var(--pico-primary);
			border-radius: 50%;
			animation: zk-spin 0.9s cubic-bezier(0.5, 0.1, 0.5, 0.9) infinite;
		}

		@keyframes zk-spin {
			0% { transform: rotate(0deg); }
			100% { transform: rotate(360deg); }
		}

		.zk-loading-content {
			display: flex;
			flex-direction: column;
			gap: 0.4rem;
			max-width: 28rem;
		}

		.zk-loading-status {
			font-size: 1.25rem;
			margin: 0;
			font-weight: 600;
		}

		.zk-loading-sub {
			font-size: 0.95rem;
			color: var(--pico-muted-color);
			margin: 0;
			line-height: 1.4;
		}

		/* Relays Table Styling */
		.relays-card {
			padding: 0;
			overflow: hidden;
			border: 1px solid var(--pico-muted-border-color);
			border-radius: var(--pico-border-radius);
			background: var(--pico-card-background-color);
			box-shadow: var(--pico-card-box-shadow);
			margin-bottom: 1.5rem;
		}

		.relays-table-wrapper {
			width: 100%;
			overflow-x: auto;
		}

		table.relays-table {
			width: 100%;
			margin: 0;
			border-collapse: collapse;
			font-size: 0.9rem;
		}

		table.relays-table th {
			background: rgba(255, 255, 255, 0.03);
			padding: 0.85rem 1rem;
			font-weight: 600;
			font-size: 0.8rem;
			text-transform: uppercase;
			letter-spacing: 0.04em;
			color: var(--pico-muted-color);
			border-bottom: 1px solid var(--pico-muted-border-color);
			white-space: nowrap;
		}

		table.relays-table td {
			padding: 0.85rem 1rem;
			vertical-align: middle;
			border-bottom: 1px solid rgba(255, 255, 255, 0.05);
		}

		table.relays-table tbody tr:last-child td {
			border-bottom: none;
		}

		table.relays-table tbody tr:hover td {
			background: rgba(255, 255, 255, 0.02);
		}

		.relay-url-box {
			display: block;
			max-width: 100%;
		}

		.relay-url {
			font-family: var(--pico-font-family-monospace, monospace);
			font-size: 0.82rem;
			word-break: break-all;
			color: var(--pico-color);
			text-decoration: none;
		}

		.relay-url:hover {
			color: var(--pico-primary);
			text-decoration: underline;
		}

		/* Badges */
		.relay-badge {
			display: inline-flex;
			align-items: center;
			gap: 0.4rem;
			padding: 0.2rem 0.6rem;
			border-radius: 9999px;
			font-size: 0.75rem;
			font-weight: 600;
			letter-spacing: 0.02em;
			white-space: nowrap;
			line-height: 1.3;
		}

		.relay-badge::before {
			content: "";
			width: 6px;
			height: 6px;
			border-radius: 50%;
			display: inline-block;
		}

		.relay-badge.online {
			background: rgba(16, 185, 129, 0.12);
			color: #10b981;
			border: 1px solid rgba(16, 185, 129, 0.28);
		}
		.relay-badge.online::before {
			background: #10b981;
			box-shadow: 0 0 6px rgba(16, 185, 129, 0.6);
		}

		.relay-badge.offline {
			background: rgba(239, 68, 68, 0.1);
			color: #ef4444;
			border: 1px solid rgba(239, 68, 68, 0.25);
		}
		.relay-badge.offline::before {
			background: #ef4444;
		}

		.relay-badge.blacklisted {
			background: rgba(245, 158, 11, 0.12);
			color: #f59e0b;
			border: 1px solid rgba(245, 158, 11, 0.28);
		}
		.relay-badge.blacklisted::before {
			background: #f59e0b;
		}

		.relay-latency-val {
			font-family: var(--pico-font-family-monospace, monospace);
			font-size: 0.85rem;
			font-weight: 500;
			color: var(--pico-color);
			white-space: nowrap;
		}

		.empty-relays {
			text-align: center;
			padding: 2.5rem 1rem !important;
		}

		/* Mobile Responsive: transform table rows into clean cards */
		@media (max-width: 680px) {
			.relays-card {
				background: transparent;
				border: none;
				box-shadow: none;
				padding: 0;
			}

			.relays-table-wrapper {
				overflow: visible;
			}

			table.relays-table thead {
				display: none;
			}

			table.relays-table,
			table.relays-table tbody {
				display: flex;
				flex-direction: column;
				gap: 0.85rem;
				width: 100%;
			}

			tr.relay-row {
				display: grid;
				grid-template-columns: 1fr auto;
				grid-template-areas:
					"status latency"
					"server server"
					"source check";
				align-items: center;
				gap: 0.55rem 0.75rem;
				padding: 1rem;
				background: var(--pico-card-background-color);
				border: 1px solid var(--pico-muted-border-color);
				border-radius: var(--pico-border-radius);
				box-shadow: var(--pico-card-box-shadow);
			}

			tr.relay-row:hover {
				border-color: rgba(248, 105, 191, 0.35);
			}

			table.relays-table td {
				padding: 0 !important;
				border: none !important;
			}

			.relay-status {
				grid-area: status;
				justify-self: start;
			}

			.relay-latency {
				grid-area: latency;
				justify-self: end;
			}

			.relay-server {
				grid-area: server;
				padding: 0.35rem 0 !important;
			}

			.relay-server .relay-url {
				font-size: 0.85rem;
				line-height: 1.4;
				display: block;
				width: 100%;
				box-sizing: border-box;
				background: rgba(0, 0, 0, 0.25);
				padding: 0.5rem 0.75rem;
				border-radius: calc(var(--pico-border-radius) / 2);
				border: 1px solid rgba(255, 255, 255, 0.06);
			}

			.relay-source {
				grid-area: source;
				justify-self: start;
				font-size: 0.78rem;
			}

			.relay-source::before {
				content: "Source: ";
				opacity: 0.7;
			}

			.relay-check {
				grid-area: check;
				justify-self: end;
				font-size: 0.78rem;
			}

			.relay-check::before {
				content: "Checked: ";
				opacity: 0.7;
			}
		}

		/* Dedicated Error Page Styles - Simple & Clean */
		.error-card {
			text-align: center;
			padding: 2.25rem 1.5rem;
			margin-bottom: 1.5rem;
			border: 1px solid var(--pico-primary);
			border-radius: var(--pico-border-radius);
			background: var(--pico-card-background-color);
			box-shadow: var(--pico-card-box-shadow);
		}

		.error-code {
			font-family: var(--pico-font-family-monospace, monospace);
			font-size: clamp(2.5rem, 8vw, 3.75rem);
			font-weight: 700;
			line-height: 1.1;
			margin: 0 0 0.5rem 0;
			color: var(--pico-color);
			user-select: none;
			-webkit-user-select: none;
		}

		.error-title {
			font-size: 1.35rem;
			font-weight: 600;
			margin: 0 0 0.75rem 0;
			color: var(--pico-color);
		}

		.error-message {
			font-size: 0.95rem;
			line-height: 1.5;
			color: var(--pico-muted-color);
			max-width: 28rem;
			margin: 0 auto;
		}

		.error-detail-box {
			margin: 1.25rem auto 0 auto;
			max-width: 30rem;
			background: rgba(0, 0, 0, 0.25);
			border: 1px solid var(--pico-muted-border-color);
			border-radius: var(--pico-border-radius);
			padding: 0.5rem 0.75rem;
			text-align: left;
		}

		.error-detail {
			font-family: var(--pico-font-family-monospace, monospace);
			font-size: 0.8rem;
			line-height: 1.4;
			color: var(--pico-muted-color);
			word-break: break-all;
			white-space: pre-wrap;
			display: block;
		}

		@media (max-width: 600px) {
			.error-card {
				padding: 1.75rem 1rem;
			}
		}

		/* Notes & Secondary notices (left-aligned) */
		.upload-note,
		.share-note,
		.relays-note,
		.error-note,
		.view-note {
			margin-top: calc(var(--pico-spacing) * 0.75);
			margin-bottom: var(--pico-spacing);
			text-align: left;
		}

		/* Site Footer */
		.site-footer {
			margin-top: calc(var(--pico-block-spacing-vertical) * 1.5);
			padding-top: var(--pico-block-spacing-vertical);
			padding-bottom: var(--pico-block-spacing-vertical);
			border-top: 1px solid var(--pico-muted-border-color);
			text-align: center;
			color: var(--pico-muted-color);
			font-size: 0.85rem;
		}

		.site-footer .footer-author {
			margin: 0;
			font-size: 0.85rem;
		}

		.upload-note a,
		.view-relays-link,
		.site-footer .footer-author a {
			font-weight: 600;
			color: var(--pico-primary);
			text-decoration: underline;
			text-decoration-color: var(--pico-primary-underline);
			transition: color var(--pico-transition), text-decoration-color var(--pico-transition);
		}

		.upload-note a:hover,
		.upload-note a:focus,
		.view-relays-link:hover,
		.view-relays-link:focus,
		.site-footer .footer-author a:hover,
		.site-footer .footer-author a:focus {
			color: var(--pico-primary-hover);
			text-decoration-color: var(--pico-primary-hover-underline);
		}
	`
)
