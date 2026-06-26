import { useCallback, useEffect, useRef, useState } from "react";
import { Button } from "@/components/ui/button";
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from "@/components/ui/dialog";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Upload, Mic, Settings, LogOut, Home, Plus, Grip, Zap, Youtube, Video, Users, MonitorSpeaker, BrainCircuit, Loader2 } from "lucide-react";
import { ScriberrLogo } from "./ScriberrLogo";
import { ThemeSwitcher } from "./ThemeSwitcher";
import { AudioRecorder } from "./AudioRecorder";
import { SystemAudioRecorder } from "./SystemAudioRecorder";
import { QuickTranscriptionDialog } from "@/features/transcription/components/QuickTranscriptionDialog";
import { YouTubeDownloadDialog } from "@/features/transcription/components/YouTubeDownloadDialog";
import { useNavigate } from "react-router-dom";
import { useAuth } from "@/features/auth/hooks/useAuth";
import { useToast } from "@/components/ui/toast";
import { isVideoFile, isAudioFile } from "../utils/fileProcessor";
import { useGlobalUpload } from "@/contexts/GlobalUploadContext";

interface FileWithType {
	file: File;
	isVideo: boolean;
}

interface HeaderProps {
	onFileSelect?: (files: File | File[] | FileWithType | FileWithType[]) => void;
	onMultiTrackClick?: () => void;
	onDownloadComplete?: () => void;
}

interface DiarizationWorkerStatus {
	state: string;
	loaded: boolean;
	model_id?: string;
	display_name?: string;
	error?: string;
}

export function Header({ onFileSelect, onMultiTrackClick, onDownloadComplete }: HeaderProps) {
	const navigate = useNavigate();
	const { logout, getAuthHeaders } = useAuth();
	const { toast } = useToast();
	const fileInputRef = useRef<HTMLInputElement>(null);
	const videoFileInputRef = useRef<HTMLInputElement>(null);
	const [isRecorderOpen, setIsRecorderOpen] = useState(false);
	const [isSystemRecorderOpen, setIsSystemRecorderOpen] = useState(false);
	const [isQuickTranscriptionOpen, setIsQuickTranscriptionOpen] = useState(false);
	const [isYouTubeDialogOpen, setIsYouTubeDialogOpen] = useState(false);
	const [isDiarizationDialogOpen, setIsDiarizationDialogOpen] = useState(false);
	const [diarizationStatus, setDiarizationStatus] = useState<DiarizationWorkerStatus>({ state: "unloaded", loaded: false });
	const [selectedDiarizationModel, setSelectedDiarizationModel] = useState("pyannote");
	const [isDiarizationActionRunning, setIsDiarizationActionRunning] = useState(false);

	// Use global upload context as fallback when props are not provided
	const globalUpload = useGlobalUpload();

	// Determine which handlers to use (prop or global context)
	const effectiveFileSelect = onFileSelect ?? globalUpload.handleFileSelect;
	const effectiveMultiTrackClick = onMultiTrackClick ?? globalUpload.openMultiTrackDialog;
	const effectiveRecordingComplete = globalUpload.handleRecordingComplete;

	const refreshDiarizationStatus = useCallback(async () => {
		try {
			const response = await fetch("/api/v1/diarization-worker/status", {
				headers: getAuthHeaders(),
			});
			if (!response.ok) {
				return null;
			}

			const status = await response.json();
			setDiarizationStatus(status);
			return status as DiarizationWorkerStatus;
		} catch {
			return null;
		}
	}, [getAuthHeaders]);

	useEffect(() => {
		refreshDiarizationStatus();
		const interval = window.setInterval(refreshDiarizationStatus, 15000);
		return () => window.clearInterval(interval);
	}, [refreshDiarizationStatus]);

	const handleUploadClick = () => {
		fileInputRef.current?.click();
	};

	const handleVideoUploadClick = () => {
		videoFileInputRef.current?.click();
	};

	const handleRecordClick = () => {
		setIsRecorderOpen(true);
	};

	const handleSystemRecordClick = () => {
		setIsSystemRecorderOpen(true);
	};

	const handleQuickTranscriptionClick = () => {
		setIsQuickTranscriptionOpen(true);
	};

	const handleYouTubeClick = () => {
		setIsYouTubeDialogOpen(true);
	};

	const handleMultiTrackClick = () => {
		effectiveMultiTrackClick();
	};

	const handleDiarizationWorkerClick = async () => {
		await refreshDiarizationStatus();
		setIsDiarizationDialogOpen(true);
	};

	const handleLoadDiarizationModel = async () => {
		setIsDiarizationActionRunning(true);
		try {
			const response = await fetch("/api/v1/diarization-worker/load", {
				method: "POST",
				headers: {
					"Content-Type": "application/json",
					...getAuthHeaders(),
				},
				body: JSON.stringify({
					model: selectedDiarizationModel,
					device: "auto",
				}),
			});
			const body = await response.json().catch(() => ({}));

			if (!response.ok) {
				if (body.status) setDiarizationStatus(body.status);
				throw new Error(body.error || "Failed to load diarization model");
			}

			setDiarizationStatus(body);
			setIsDiarizationDialogOpen(false);
			toast({ title: `${body.display_name || "Diarization model"} loaded` });
		} catch (error) {
			toast({
				title: "Could not load diarization model",
				description: error instanceof Error ? error.message : "Unknown error",
			});
		} finally {
			setIsDiarizationActionRunning(false);
		}
	};

	const handleUnloadDiarizationModel = async () => {
		setIsDiarizationActionRunning(true);
		try {
			const response = await fetch("/api/v1/diarization-worker/unload", {
				method: "POST",
				headers: getAuthHeaders(),
			});
			const body = await response.json().catch(() => ({}));

			if (!response.ok) {
				if (body.status) setDiarizationStatus(body.status);
				throw new Error(body.error || "Failed to unload diarization model");
			}

			setDiarizationStatus(body);
			setIsDiarizationDialogOpen(false);
			toast({ title: "Diarization model unloaded" });
		} catch (error) {
			toast({
				title: "Could not unload diarization model",
				description: error instanceof Error ? error.message : "Unknown error",
			});
		} finally {
			setIsDiarizationActionRunning(false);
		}
	};

	const handleSettingsClick = () => {
		navigate("/settings");
	};

	const handleLogout = () => {
		logout();
	};

	const handleHomeClick = () => {
		navigate("/");
	};

	const handleFileChange = (event: React.ChangeEvent<HTMLInputElement>) => {
		const files = event.target.files;
		if (files && files.length > 0) {
			const fileArray = Array.from(files);

			// Check for video files that were incorrectly uploaded via audio upload
			const videoFiles = fileArray.filter(file => isVideoFile(file));
			if (videoFiles.length > 0) {
				alert(`Video files detected. Please use "Upload Videos" instead of "Upload Files" to upload ${videoFiles.map(f => f.name).join(', ')}`);
				event.target.value = "";
				return;
			}

			// Filter to only audio files
			const audioFiles = fileArray.filter(file => isAudioFile(file));
			if (audioFiles.length > 0) {
				effectiveFileSelect(audioFiles.length === 1 ? audioFiles[0] : audioFiles);
				// Reset the input so the same files can be selected again
				event.target.value = "";
			} else {
				// No valid audio files found
				alert("No valid audio files found. Please select audio files (.mp3, .wav, .flac, .m4a, .aac, .ogg)");
				event.target.value = "";
			}
		}
	};

	const handleVideoFileChange = (event: React.ChangeEvent<HTMLInputElement>) => {
		const files = event.target.files;
		if (files && files.length > 0) {
			// Filter to only video files
			const videoFiles = Array.from(files).filter(file => file.type.startsWith("video/"));
			if (videoFiles.length > 0) {
				// Pass video files with type marker
				const filesWithType: FileWithType[] = videoFiles.map(file => ({ file, isVideo: true }));
				effectiveFileSelect(filesWithType.length === 1 ? filesWithType[0] : filesWithType);
				// Reset the input so the same files can be selected again
				event.target.value = "";
			}
		}
	};

	const handleRecordingComplete = async (blob: Blob, title: string) => {
		// Use global recording complete handler
		await effectiveRecordingComplete(blob, title);
	};

	const isDiarizationBusy =
		isDiarizationActionRunning ||
		diarizationStatus.state === "loading" ||
		diarizationStatus.state === "unloading";
	const diarizationLabel = diarizationStatus.display_name || "Diarization model";

	return (
		<header className="sticky top-4 sm:top-6 z-50 glass rounded-[var(--radius-card)] px-4 py-3 sm:px-6 sm:py-4 transition-all duration-500 shadow-[var(--shadow-float)] border border-[var(--border-subtle)]">
			<div className="flex items-center justify-between">
				{/* Left side - Logo navigates home */}
				<ScriberrLogo onClick={handleHomeClick} />

				{/* Right side - Plus (Add Audio), Grip Menu, Theme Switcher */}
				<div className="flex items-center gap-2 sm:gap-3">
					{/* Add Audio (icon-only) */}
					<DropdownMenu>
						<DropdownMenuTrigger asChild>
							<Button
								variant="default"
								size="icon"
								className="bg-gradient-to-br from-[#FFAB40] to-[#FF3D00] text-white shadow-[0_4px_12px_rgba(255,61,0,0.4)] hover:shadow-[0_6px_16px_rgba(255,61,0,0.5)] border-none h-8 w-8 sm:h-10 sm:w-10 rounded-lg transition-all hover:scale-105 active:scale-95 cursor-pointer"
							>
								<Plus className="h-5 w-5 sm:h-6 sm:w-6" />
								<span className="sr-only">Add audio</span>
							</Button>
						</DropdownMenuTrigger>
						<DropdownMenuContent
							align="end"
							className="w-64 glass-card p-2 rounded-[var(--radius-card)] shadow-[var(--shadow-float)] border-[var(--border-subtle)]"
						>
							<DropdownMenuItem
								onClick={handleQuickTranscriptionClick}
								className="group flex items-center gap-3 px-3 py-3 cursor-pointer rounded-[var(--radius-btn)] focus:bg-[var(--brand-light)] focus:text-[var(--brand-solid)] transition-colors"
							>
								<div className="p-2 bg-amber-500/10 rounded-[var(--radius-btn)] text-amber-600 group-focus:text-[var(--brand-solid)]">
									<Zap className="h-4 w-4" />
								</div>
								<div>
									<div className="font-medium text-sm">Quick Transcribe</div>
									<div className="text-xs text-[var(--text-secondary)]">
										Fast transcribe without saving
									</div>
								</div>
							</DropdownMenuItem>
							<DropdownMenuItem
								onClick={handleYouTubeClick}
								className="group flex items-center gap-3 px-3 py-3 cursor-pointer rounded-[var(--radius-btn)] focus:bg-[var(--brand-light)] focus:text-[var(--brand-solid)] transition-colors"
							>
								<div className="p-2 bg-rose-500/10 rounded-[var(--radius-btn)] text-rose-600 group-focus:text-[var(--brand-solid)]">
									<Youtube className="h-4 w-4" />
								</div>
								<div>
									<div className="font-medium text-sm">YouTube URL</div>
									<div className="text-xs text-[var(--text-secondary)]">
										Download audio from YouTube
									</div>
								</div>
							</DropdownMenuItem>
							<DropdownMenuItem
								onClick={handleUploadClick}
								className="group flex items-center gap-3 px-3 py-3 cursor-pointer rounded-[var(--radius-btn)] focus:bg-[var(--brand-light)] focus:text-[var(--brand-solid)] transition-colors"
							>
								<div className="p-2 bg-[var(--brand-light)] rounded-[var(--radius-btn)] text-[var(--brand-solid)] group-focus:text-[var(--brand-solid)]">
									<Upload className="h-4 w-4" />
								</div>
								<div>
									<div className="font-medium text-sm">Upload Files</div>
									<div className="text-xs text-[var(--text-secondary)]">
										Choose one or more audio files
									</div>
								</div>
							</DropdownMenuItem>
							<DropdownMenuItem
								onClick={handleVideoUploadClick}
								className="group flex items-center gap-3 px-3 py-3 cursor-pointer rounded-[var(--radius-btn)] focus:bg-[var(--brand-light)] focus:text-[var(--brand-solid)] transition-colors"
							>
								<div className="p-2 bg-purple-500/10 rounded-[var(--radius-btn)] text-purple-600 group-focus:text-[var(--brand-solid)]">
									<Video className="h-4 w-4" />
								</div>
								<div>
									<div className="font-medium text-sm">Upload Videos</div>
									<div className="text-xs text-[var(--text-secondary)]">
										Extract audio from video files
									</div>
								</div>
							</DropdownMenuItem>
							<DropdownMenuItem
								onClick={handleRecordClick}
								className="group flex items-center gap-3 px-3 py-3 cursor-pointer rounded-[var(--radius-btn)] focus:bg-[var(--brand-light)] focus:text-[var(--brand-solid)] transition-colors"
							>
								<div className="p-2 bg-emerald-500/10 rounded-[var(--radius-btn)] text-emerald-600 group-focus:text-[var(--brand-solid)]">
									<Mic className="h-4 w-4" />
								</div>
								<div>
									<div className="font-medium text-sm">Record Audio</div>
									<div className="text-xs text-[var(--text-secondary)]">
										Record using microphone
									</div>
								</div>
							</DropdownMenuItem>
							<DropdownMenuItem
								onClick={handleSystemRecordClick}
								className="group flex items-center gap-3 px-3 py-3 cursor-pointer rounded-[var(--radius-btn)] focus:bg-[var(--brand-light)] focus:text-[var(--brand-solid)] transition-colors"
							>
								<div className="p-2 bg-blue-500/10 rounded-[var(--radius-btn)] text-blue-600 group-focus:text-[var(--brand-solid)]">
									<MonitorSpeaker className="h-4 w-4" />
								</div>
								<div>
									<div className="font-medium text-sm">Record System Audio</div>
									<div className="text-xs text-[var(--text-secondary)]">
										Capture screen + microphone
									</div>
								</div>
							</DropdownMenuItem>
							<DropdownMenuItem
								onClick={handleMultiTrackClick}
								className="group flex items-center gap-3 px-3 py-3 cursor-pointer rounded-[var(--radius-btn)] focus:bg-[var(--brand-light)] focus:text-[var(--brand-solid)] transition-colors"
							>
								<div className="p-2 bg-indigo-500/10 rounded-[var(--radius-btn)] text-indigo-600 group-focus:text-[var(--brand-solid)]">
									<Users className="h-4 w-4" />
								</div>
								<div>
									<div className="font-medium text-sm">Multi-Track Audio</div>
									<div className="text-xs text-[var(--text-secondary)]">
										Upload multiple speaker tracks
									</div>
								</div>
							</DropdownMenuItem>
						</DropdownMenuContent>
					</DropdownMenu>

					<Button
						variant="ghost"
						size="icon"
						title={diarizationStatus.loaded ? `${diarizationLabel} loaded` : "Load diarization model"}
						onClick={handleDiarizationWorkerClick}
						disabled={isDiarizationBusy}
						className={`relative h-8 w-8 sm:h-10 sm:w-10 rounded-[var(--radius-btn)] cursor-pointer ${
							diarizationStatus.loaded
								? "bg-emerald-500/10 text-emerald-600 hover:bg-emerald-500/15"
								: "text-[var(--text-secondary)] hover:bg-[var(--secondary)]"
						}`}
					>
						{isDiarizationBusy ? (
							<Loader2 className="h-4 w-4 sm:h-5 sm:w-5 animate-spin" />
						) : (
							<BrainCircuit className="h-4 w-4 sm:h-5 sm:w-5" />
						)}
						{diarizationStatus.loaded && (
							<span className="absolute right-1 top-1 h-2 w-2 rounded-full bg-emerald-500 ring-2 ring-[var(--bg-card)]" />
						)}
						<span className="sr-only">
							{diarizationStatus.loaded ? "Unload diarization model" : "Load diarization model"}
						</span>
					</Button>

					{/* Main Menu (Grip) */}
					<DropdownMenu>
						<DropdownMenuTrigger asChild>
							<Button
								variant="ghost"
								size="icon"
								className="h-8 w-8 sm:h-10 sm:w-10 hover:bg-[var(--secondary)] rounded-[var(--radius-btn)] cursor-pointer text-[var(--text-secondary)]"
							>
								<Grip className="h-4 w-4 sm:h-5 sm:w-5" />
								<span className="sr-only">Open menu</span>
							</Button>
						</DropdownMenuTrigger>
						<DropdownMenuContent align="end" className="w-48 glass-card border-[var(--border-subtle)] p-2 rounded-[var(--radius-card)] shadow-[var(--shadow-float)]">
							<DropdownMenuItem onClick={handleHomeClick} className="cursor-pointer rounded-[var(--radius-btn)] focus:bg-[var(--secondary)] py-2.5">
								<Home className="h-4 w-4 mr-2" />
								Home
							</DropdownMenuItem>
							<DropdownMenuItem onClick={handleSettingsClick} className="cursor-pointer rounded-[var(--radius-btn)] focus:bg-[var(--secondary)] py-2.5">
								<Settings className="h-4 w-4 mr-2" />
								Settings
							</DropdownMenuItem>
							<DropdownMenuItem onClick={handleLogout} className="cursor-pointer rounded-[var(--radius-btn)] focus:bg-[var(--error)]/10 text-[var(--error)] py-2.5">
								<LogOut className="h-4 w-4 mr-2" />
								Logout
							</DropdownMenuItem>
						</DropdownMenuContent>
					</DropdownMenu>

					{/* Theme Switcher (icon-only) */}
					<ThemeSwitcher />

					{/* Hidden file input */}
					<input
						ref={fileInputRef}
						type="file"
						accept="audio/*"
						multiple
						onChange={handleFileChange}
						className="hidden"
					/>

					{/* Hidden video file input */}
					<input
						ref={videoFileInputRef}
						type="file"
						accept="video/*"
						multiple
						onChange={handleVideoFileChange}
						className="hidden"
					/>
				</div>
			</div>

			{/* Audio Recorder Dialog */}
			<AudioRecorder
				isOpen={isRecorderOpen}
				onClose={() => setIsRecorderOpen(false)}
				onRecordingComplete={handleRecordingComplete}
			/>

			{/* System Audio Recorder Dialog */}
			<SystemAudioRecorder
				isOpen={isSystemRecorderOpen}
				onClose={() => setIsSystemRecorderOpen(false)}
				onRecordingComplete={effectiveRecordingComplete}
			/>

			{/* Quick Transcription Dialog */}
			<QuickTranscriptionDialog
				isOpen={isQuickTranscriptionOpen}
				onClose={() => setIsQuickTranscriptionOpen(false)}
			/>

			{/* YouTube Download Dialog */}
			<YouTubeDownloadDialog
				isOpen={isYouTubeDialogOpen}
				onClose={() => setIsYouTubeDialogOpen(false)}
				onDownloadComplete={onDownloadComplete}
			/>

			<Dialog open={isDiarizationDialogOpen} onOpenChange={setIsDiarizationDialogOpen}>
				<DialogContent className="bg-[var(--bg-card)] border border-[var(--border-subtle)] text-[var(--text-primary)]">
					{diarizationStatus.loaded ? (
						<>
							<DialogHeader>
								<DialogTitle>Unload diarization model?</DialogTitle>
								<DialogDescription>
									{diarizationLabel} is currently loaded in GPU memory.
								</DialogDescription>
							</DialogHeader>
							<DialogFooter>
								<Button
									variant="ghost"
									onClick={() => setIsDiarizationDialogOpen(false)}
									disabled={isDiarizationActionRunning}
								>
									Cancel
								</Button>
								<Button
									variant="destructive"
									onClick={handleUnloadDiarizationModel}
									disabled={isDiarizationActionRunning}
								>
									{isDiarizationActionRunning && <Loader2 className="h-4 w-4 animate-spin" />}
									Unload
								</Button>
							</DialogFooter>
						</>
					) : (
						<>
							<DialogHeader>
								<DialogTitle>Load diarization model</DialogTitle>
								<DialogDescription>
									Choose a diarization model to keep loaded in GPU memory.
								</DialogDescription>
							</DialogHeader>
							<div className="space-y-2">
								<Select value={selectedDiarizationModel} onValueChange={setSelectedDiarizationModel}>
									<SelectTrigger className="w-full bg-[var(--bg-main)] border-[var(--border-subtle)] text-[var(--text-primary)]">
										<SelectValue placeholder="Choose a model" />
									</SelectTrigger>
									<SelectContent className="bg-[var(--bg-card)] border-[var(--border-subtle)] text-[var(--text-primary)]">
										<SelectItem value="pyannote" className="focus:bg-[var(--bg-secondary)] focus:text-[var(--text-primary)]">PyAnnote</SelectItem>
										<SelectItem value="sortformer" className="focus:bg-[var(--bg-secondary)] focus:text-[var(--text-primary)]">Sortformer</SelectItem>
									</SelectContent>
								</Select>
								{diarizationStatus.state === "failed" && diarizationStatus.error && (
									<p className="text-sm text-[var(--error)]">{diarizationStatus.error}</p>
								)}
							</div>
							<DialogFooter>
								<Button
									variant="ghost"
									onClick={() => setIsDiarizationDialogOpen(false)}
									disabled={isDiarizationActionRunning}
								>
									Cancel
								</Button>
								<Button
									onClick={handleLoadDiarizationModel}
									disabled={isDiarizationActionRunning}
								>
									{isDiarizationActionRunning && <Loader2 className="h-4 w-4 animate-spin" />}
									Load
								</Button>
							</DialogFooter>
						</>
					)}
				</DialogContent>
			</Dialog>

		</header>
	);
}
