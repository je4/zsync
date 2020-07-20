package zotero

type ItemGeneric struct {
	ItemDataBase
	NumPages             string `json:"numPages,omitempty"`             // # of Pages
	NumberOfVolumes      string `json:"numberOfVolumes,omitempty"`      // # of Volumes
	AbstractNote         string `json:"abstractNote,omitempty"`         // Abstract
	AccessDate           string `json:"accessDate,omitempty"`           // Accessed
	ApplicationNumber    string `json:"applicationNumber,omitempty"`    // Application Number
	Archive              string `json:"archive,omitempty"`              // Archive
	ArtworkSize          string `json:"artworkSize,omitempty"`          // Artwork Size
	Assignee             string `json:"assignee,omitempty"`             // Assignee
	BillNumber           string `json:"billNumber,omitempty"`           // Bill Number
	BlogTitle            string `json:"blogTitle,omitempty"`            // Blog Title
	BookTitle            string `json:"bookTitle,omitempty"`            // Book Title
	CallNumber           string `json:"callNumber,omitempty"`           // Call Number
	CaseName             string `json:"caseName,omitempty"`             // Case Name
	Code                 string `json:"code,omitempty"`                 // Code
	CodeNumber           string `json:"codeNumber,omitempty"`           // Code Number
	CodePages            string `json:"codePages,omitempty"`            // Code Pages
	CodeVolume           string `json:"codeVolume,omitempty"`           // Code Volume
	Committee            string `json:"committee,omitempty"`            // Committee
	Company              string `json:"company,omitempty"`              // Company
	ConferenceName       string `json:"conferenceName,omitempty"`       // Conference Name
	Country              string `json:"country,omitempty"`              // Country
	Court                string `json:"court,omitempty"`                // Court
	DOI                  string `json:"DOI,omitempty"`                  // DOI
	Date                 string `json:"date,omitempty"`                 // Date
	DateDecided          string `json:"dateDecided,omitempty"`          // Date Decided
	DateEnacted          string `json:"dateEnacted,omitempty"`          // Date Enacted
	DictionaryTitle      string `json:"dictionaryTitle,omitempty"`      // Dictionary Title
	Distributor          string `json:"distributor,omitempty"`          // Distributor
	DocketNumber         string `json:"docketNumber,omitempty"`         // Docket Number
	DocumentNumber       string `json:"documentNumber,omitempty"`       // Document Number
	Edition              string `json:"edition,omitempty"`              // Edition
	EncyclopediaTitle    string `json:"encyclopediaTitle,omitempty"`    // Encyclopedia Title
	EpisodeNumber        string `json:"episodeNumber,omitempty"`        // Episode Number
	Extra                string `json:"extra,omitempty"`                // Extra
	AudioFileType        string `json:"audioFileType,omitempty"`        // File Type
	FilingDate           string `json:"filingDate,omitempty"`           // Filing Date
	FirstPage            string `json:"firstPage,omitempty"`            // First Page
	AudioRecordingFormat string `json:"audioRecordingFormat,omitempty"` // Format
	VideoRecordingFormat string `json:"videoRecordingFormat,omitempty"` // Format
	ForumTitle           string `json:"forumTitle,omitempty"`           // Forum/Listserv Title
	Genre                string `json:"genre,omitempty"`                // Genre
	History              string `json:"history,omitempty"`              // History
	ISBN                 string `json:"ISBN,omitempty"`                 // ISBN
	ISSN                 string `json:"ISSN,omitempty"`                 // ISSN
	Institution          string `json:"institution,omitempty"`          // Institution
	Issue                string `json:"issue,omitempty"`                // Issue
	IssueDate            string `json:"issueDate,omitempty"`            // Issue Date
	IssuingAuthority     string `json:"issuingAuthority,omitempty"`     // Issuing Authority
	JournalAbbreviation  string `json:"journalAbbreviation,omitempty"`  // Journal Abbr
	Label                string `json:"label,omitempty"`                // Label
	Language             string `json:"language,omitempty"`             // Language
	ProgrammingLanguage  string `json:"programmingLanguage,omitempty"`  // Language
	LegalStatus          string `json:"legalStatus,omitempty"`          // Legal Status
	LegislativeBody      string `json:"legislativeBody,omitempty"`      // Legislative Body
	LibraryCatalog       string `json:"libraryCatalog,omitempty"`       // Library Catalog
	ArchiveLocation      string `json:"archiveLocation,omitempty"`      // Loc. in Archive
	InterviewMedium      string `json:"interviewMedium,omitempty"`      // Medium
	ArtworkMedium        string `json:"artworkMedium,omitempty"`        // Medium
	MeetingName          string `json:"meetingName,omitempty"`          // Meeting Name
	NameOfAct            string `json:"nameOfAct,omitempty"`            // Name of Act
	Network              string `json:"network,omitempty"`              // Network
	Pages                string `json:"pages,omitempty"`                // Pages
	PatentNumber         string `json:"patentNumber,omitempty"`         // Patent Number
	Place                string `json:"place,omitempty"`                // Place
	PostType             string `json:"postType,omitempty"`             // Post Type
	PriorityNumbers      string `json:"priorityNumbers,omitempty"`      // Priority Numbers
	ProceedingsTitle     string `json:"proceedingsTitle,omitempty"`     // Proceedings Title
	ProgramTitle         string `json:"programTitle,omitempty"`         // Program Title
	PublicLawNumber      string `json:"publicLawNumber,omitempty"`      // Public Law Number
	PublicationTitle     string `json:"publicationTitle,omitempty"`     // Publication
	Publisher            string `json:"publisher,omitempty"`            // Publisher
	References           string `json:"references,omitempty"`           // References
	ReportNumber         string `json:"reportNumber,omitempty"`         // Report Number
	ReportType           string `json:"reportType,omitempty"`           // Report Type
	Reporter             string `json:"reporter,omitempty"`             // Reporter
	ReporterVolume       string `json:"reporterVolume,omitempty"`       // Reporter Volume
	Rights               string `json:"rights,omitempty"`               // Rights
	RunningTime          string `json:"runningTime,omitempty"`          // Running Time
	Scale                string `json:"scale,omitempty"`                // Scale
	Section              string `json:"section,omitempty"`              // Section
	Series               string `json:"series,omitempty"`               // Series
	SeriesNumber         string `json:"seriesNumber,omitempty"`         // Series Number
	SeriesText           string `json:"seriesText,omitempty"`           // Series Text
	SeriesTitle          string `json:"seriesTitle,omitempty"`          // Series Title
	Session              string `json:"session,omitempty"`              // Session
	ShortTitle           string `json:"shortTitle,omitempty"`           // Short Title
	Studio               string `json:"studio,omitempty"`               // Studio
	Subject              string `json:"subject,omitempty"`              // Subject
	System               string `json:"system,omitempty"`               // System
	Title                string `json:"title,omitempty"`                // Title
	ThesisType           string `json:"thesisType,omitempty"`           // Type
	PresentationType     string `json:"presentationType,omitempty"`     // Type
	MapType              string `json:"mapType,omitempty"`              // Type
	ManuscriptType       string `json:"manuscriptType,omitempty"`       // Type
	LetterType           string `json:"letterType,omitempty"`           // Type
	Url                  string `json:"url,omitempty"`                  // URL
	University           string `json:"university,omitempty"`           // University
	VersionNumber        string `json:"versionNumber,omitempty"`        // Version
	Volume               string `json:"volume,omitempty"`               // Volume
	WebsiteTitle         string `json:"websiteTitle,omitempty"`         // Website Title
	WebsiteType          string `json:"websiteType,omitempty"`          // Website Type

	// Attachment
	LinkMode    string `json:"linkMode,omitempty"`
	Note        string `json:"note,omitempty"`
	ContentType string `json:"contentType,omitempty"`
	Charset     string `json:"charset,omitempty"`
	Filename    string `json:"filename,omitempty"`
	MD5         string `json:"md5,omitempty"`
	MTime       int64  `json:"mtime,omitempty"`
}

type ItemArtwork struct {
	ItemDataBase
	Title           string `json:"title,omitempty"`           // Title
	AbstractNote    string `json:"abstractNote,omitempty"`    // Abstract
	ArtworkMedium   string `json:"artworkMedium,omitempty"`   // Medium
	ArtworkSize     string `json:"artworkSize,omitempty"`     // Artwork Size
	Date            string `json:"date,omitempty"`            // Date
	Language        string `json:"language,omitempty"`        // Language
	ShortTitle      string `json:"shortTitle,omitempty"`      // Short Title
	Archive         string `json:"archive,omitempty"`         // Archive
	ArchiveLocation string `json:"archiveLocation,omitempty"` // Loc. in Archive
	LibraryCatalog  string `json:"libraryCatalog,omitempty"`  // Library Catalog
	CallNumber      string `json:"callNumber,omitempty"`      // Call Number
	Url             string `json:"url,omitempty"`             // URL
	AccessDate      string `json:"accessDate,omitempty"`      // Accessed
	Rights          string `json:"rights,omitempty"`          // Rights
	Extra           string `json:"extra,omitempty"`           // Extra
}

type ItemAudioRecording struct {
	ItemDataBase
	Title                string `json:"title,omitempty"`                // Title
	AbstractNote         string `json:"abstractNote,omitempty"`         // Abstract
	AudioRecordingFormat string `json:"audioRecordingFormat,omitempty"` // Format
	SeriesTitle          string `json:"seriesTitle,omitempty"`          // Series Title
	Volume               string `json:"volume,omitempty"`               // Volume
	NumberOfVolumes      string `json:"numberOfVolumes,omitempty"`      // # of Volumes
	Place                string `json:"place,omitempty"`                // Place
	Label                string `json:"label,omitempty"`                // Label
	Date                 string `json:"date,omitempty"`                 // Date
	RunningTime          string `json:"runningTime,omitempty"`          // Running Time
	Language             string `json:"language,omitempty"`             // Language
	ISBN                 string `json:"ISBN,omitempty"`                 // ISBN
	ShortTitle           string `json:"shortTitle,omitempty"`           // Short Title
	Archive              string `json:"archive,omitempty"`              // Archive
	ArchiveLocation      string `json:"archiveLocation,omitempty"`      // Loc. in Archive
	LibraryCatalog       string `json:"libraryCatalog,omitempty"`       // Library Catalog
	CallNumber           string `json:"callNumber,omitempty"`           // Call Number
	Url                  string `json:"url,omitempty"`                  // URL
	AccessDate           string `json:"accessDate,omitempty"`           // Accessed
	Rights               string `json:"rights,omitempty"`               // Rights
	Extra                string `json:"extra,omitempty"`                // Extra
}

type ItemBill struct {
	ItemDataBase
	Title           string `json:"title,omitempty"`           // Title
	AbstractNote    string `json:"abstractNote,omitempty"`    // Abstract
	BillNumber      string `json:"billNumber,omitempty"`      // Bill Number
	Code            string `json:"code,omitempty"`            // Code
	CodeVolume      string `json:"codeVolume,omitempty"`      // Code Volume
	Section         string `json:"section,omitempty"`         // Section
	CodePages       string `json:"codePages,omitempty"`       // Code Pages
	LegislativeBody string `json:"legislativeBody,omitempty"` // Legislative Body
	Session         string `json:"session,omitempty"`         // Session
	History         string `json:"history,omitempty"`         // History
	Date            string `json:"date,omitempty"`            // Date
	Language        string `json:"language,omitempty"`        // Language
	Url             string `json:"url,omitempty"`             // URL
	AccessDate      string `json:"accessDate,omitempty"`      // Accessed
	ShortTitle      string `json:"shortTitle,omitempty"`      // Short Title
	Rights          string `json:"rights,omitempty"`          // Rights
	Extra           string `json:"extra,omitempty"`           // Extra
}

type ItemBlogPost struct {
	ItemDataBase
	Title        string `json:"title,omitempty"`        // Title
	AbstractNote string `json:"abstractNote,omitempty"` // Abstract
	BlogTitle    string `json:"blogTitle,omitempty"`    // Blog Title
	WebsiteType  string `json:"websiteType,omitempty"`  // Website Type
	Date         string `json:"date,omitempty"`         // Date
	Url          string `json:"url,omitempty"`          // URL
	AccessDate   string `json:"accessDate,omitempty"`   // Accessed
	Language     string `json:"language,omitempty"`     // Language
	ShortTitle   string `json:"shortTitle,omitempty"`   // Short Title
	Rights       string `json:"rights,omitempty"`       // Rights
	Extra        string `json:"extra,omitempty"`        // Extra
}

type ItemBook struct {
	ItemDataBase
	Title           string `json:"title,omitempty"`           // Title
	AbstractNote    string `json:"abstractNote,omitempty"`    // Abstract
	Series          string `json:"series,omitempty"`          // Series
	SeriesNumber    string `json:"seriesNumber,omitempty"`    // Series Number
	Volume          string `json:"volume,omitempty"`          // Volume
	NumberOfVolumes string `json:"numberOfVolumes,omitempty"` // # of Volumes
	Edition         string `json:"edition,omitempty"`         // Edition
	Place           string `json:"place,omitempty"`           // Place
	Publisher       string `json:"publisher,omitempty"`       // Publisher
	Date            string `json:"date,omitempty"`            // Date
	NumPages        string `json:"numPages,omitempty"`        // # of Pages
	Language        string `json:"language,omitempty"`        // Language
	ISBN            string `json:"ISBN,omitempty"`            // ISBN
	ShortTitle      string `json:"shortTitle,omitempty"`      // Short Title
	Url             string `json:"url,omitempty"`             // URL
	AccessDate      string `json:"accessDate,omitempty"`      // Accessed
	Archive         string `json:"archive,omitempty"`         // Archive
	ArchiveLocation string `json:"archiveLocation,omitempty"` // Loc. in Archive
	LibraryCatalog  string `json:"libraryCatalog,omitempty"`  // Library Catalog
	CallNumber      string `json:"callNumber,omitempty"`      // Call Number
	Rights          string `json:"rights,omitempty"`          // Rights
	Extra           string `json:"extra,omitempty"`           // Extra
}

type ItemBookSection struct {
	ItemDataBase
	Title           string `json:"title,omitempty"`           // Title
	AbstractNote    string `json:"abstractNote,omitempty"`    // Abstract
	BookTitle       string `json:"bookTitle,omitempty"`       // Book Title
	Series          string `json:"series,omitempty"`          // Series
	SeriesNumber    string `json:"seriesNumber,omitempty"`    // Series Number
	Volume          string `json:"volume,omitempty"`          // Volume
	NumberOfVolumes string `json:"numberOfVolumes,omitempty"` // # of Volumes
	Edition         string `json:"edition,omitempty"`         // Edition
	Place           string `json:"place,omitempty"`           // Place
	Publisher       string `json:"publisher,omitempty"`       // Publisher
	Date            string `json:"date,omitempty"`            // Date
	Pages           string `json:"pages,omitempty"`           // Pages
	Language        string `json:"language,omitempty"`        // Language
	ISBN            string `json:"ISBN,omitempty"`            // ISBN
	ShortTitle      string `json:"shortTitle,omitempty"`      // Short Title
	Url             string `json:"url,omitempty"`             // URL
	AccessDate      string `json:"accessDate,omitempty"`      // Accessed
	Archive         string `json:"archive,omitempty"`         // Archive
	ArchiveLocation string `json:"archiveLocation,omitempty"` // Loc. in Archive
	LibraryCatalog  string `json:"libraryCatalog,omitempty"`  // Library Catalog
	CallNumber      string `json:"callNumber,omitempty"`      // Call Number
	Rights          string `json:"rights,omitempty"`          // Rights
	Extra           string `json:"extra,omitempty"`           // Extra
}

type ItemCase struct {
	ItemDataBase
	CaseName       string `json:"caseName,omitempty"`       // Case Name
	AbstractNote   string `json:"abstractNote,omitempty"`   // Abstract
	Reporter       string `json:"reporter,omitempty"`       // Reporter
	ReporterVolume string `json:"reporterVolume,omitempty"` // Reporter Volume
	Court          string `json:"court,omitempty"`          // Court
	DocketNumber   string `json:"docketNumber,omitempty"`   // Docket Number
	FirstPage      string `json:"firstPage,omitempty"`      // First Page
	History        string `json:"history,omitempty"`        // History
	DateDecided    string `json:"dateDecided,omitempty"`    // Date Decided
	Language       string `json:"language,omitempty"`       // Language
	ShortTitle     string `json:"shortTitle,omitempty"`     // Short Title
	Url            string `json:"url,omitempty"`            // URL
	AccessDate     string `json:"accessDate,omitempty"`     // Accessed
	Rights         string `json:"rights,omitempty"`         // Rights
	Extra          string `json:"extra,omitempty"`          // Extra
}

type ItemComputerProgram struct {
	ItemDataBase
	Title               string `json:"title,omitempty"`               // Title
	AbstractNote        string `json:"abstractNote,omitempty"`        // Abstract
	SeriesTitle         string `json:"seriesTitle,omitempty"`         // Series Title
	VersionNumber       string `json:"versionNumber,omitempty"`       // Version
	Date                string `json:"date,omitempty"`                // Date
	System              string `json:"system,omitempty"`              // System
	Place               string `json:"place,omitempty"`               // Place
	Company             string `json:"company,omitempty"`             // Company
	ProgrammingLanguage string `json:"programmingLanguage,omitempty"` // Language
	ISBN                string `json:"ISBN,omitempty"`                // ISBN
	ShortTitle          string `json:"shortTitle,omitempty"`          // Short Title
	Url                 string `json:"url,omitempty"`                 // URL
	Rights              string `json:"rights,omitempty"`              // Rights
	Archive             string `json:"archive,omitempty"`             // Archive
	ArchiveLocation     string `json:"archiveLocation,omitempty"`     // Loc. in Archive
	LibraryCatalog      string `json:"libraryCatalog,omitempty"`      // Library Catalog
	CallNumber          string `json:"callNumber,omitempty"`          // Call Number
	AccessDate          string `json:"accessDate,omitempty"`          // Accessed
	Extra               string `json:"extra,omitempty"`               // Extra
}

type ItemConferencePaper struct {
	ItemDataBase
	Title            string `json:"title,omitempty"`            // Title
	AbstractNote     string `json:"abstractNote,omitempty"`     // Abstract
	Date             string `json:"date,omitempty"`             // Date
	ProceedingsTitle string `json:"proceedingsTitle,omitempty"` // Proceedings Title
	ConferenceName   string `json:"conferenceName,omitempty"`   // Conference Name
	Place            string `json:"place,omitempty"`            // Place
	Publisher        string `json:"publisher,omitempty"`        // Publisher
	Volume           string `json:"volume,omitempty"`           // Volume
	Pages            string `json:"pages,omitempty"`            // Pages
	Series           string `json:"series,omitempty"`           // Series
	Language         string `json:"language,omitempty"`         // Language
	DOI              string `json:"DOI,omitempty"`              // DOI
	ISBN             string `json:"ISBN,omitempty"`             // ISBN
	ShortTitle       string `json:"shortTitle,omitempty"`       // Short Title
	Url              string `json:"url,omitempty"`              // URL
	AccessDate       string `json:"accessDate,omitempty"`       // Accessed
	Archive          string `json:"archive,omitempty"`          // Archive
	ArchiveLocation  string `json:"archiveLocation,omitempty"`  // Loc. in Archive
	LibraryCatalog   string `json:"libraryCatalog,omitempty"`   // Library Catalog
	CallNumber       string `json:"callNumber,omitempty"`       // Call Number
	Rights           string `json:"rights,omitempty"`           // Rights
	Extra            string `json:"extra,omitempty"`            // Extra
}

type ItemDictionaryEntry struct {
	ItemDataBase
	Title           string `json:"title,omitempty"`           // Title
	AbstractNote    string `json:"abstractNote,omitempty"`    // Abstract
	DictionaryTitle string `json:"dictionaryTitle,omitempty"` // Dictionary Title
	Series          string `json:"series,omitempty"`          // Series
	SeriesNumber    string `json:"seriesNumber,omitempty"`    // Series Number
	Volume          string `json:"volume,omitempty"`          // Volume
	NumberOfVolumes string `json:"numberOfVolumes,omitempty"` // # of Volumes
	Edition         string `json:"edition,omitempty"`         // Edition
	Place           string `json:"place,omitempty"`           // Place
	Publisher       string `json:"publisher,omitempty"`       // Publisher
	Date            string `json:"date,omitempty"`            // Date
	Pages           string `json:"pages,omitempty"`           // Pages
	Language        string `json:"language,omitempty"`        // Language
	ISBN            string `json:"ISBN,omitempty"`            // ISBN
	ShortTitle      string `json:"shortTitle,omitempty"`      // Short Title
	Url             string `json:"url,omitempty"`             // URL
	AccessDate      string `json:"accessDate,omitempty"`      // Accessed
	Archive         string `json:"archive,omitempty"`         // Archive
	ArchiveLocation string `json:"archiveLocation,omitempty"` // Loc. in Archive
	LibraryCatalog  string `json:"libraryCatalog,omitempty"`  // Library Catalog
	CallNumber      string `json:"callNumber,omitempty"`      // Call Number
	Rights          string `json:"rights,omitempty"`          // Rights
	Extra           string `json:"extra,omitempty"`           // Extra
}

type ItemDocument struct {
	ItemDataBase
	Title           string `json:"title,omitempty"`           // Title
	AbstractNote    string `json:"abstractNote,omitempty"`    // Abstract
	Publisher       string `json:"publisher,omitempty"`       // Publisher
	Date            string `json:"date,omitempty"`            // Date
	Language        string `json:"language,omitempty"`        // Language
	ShortTitle      string `json:"shortTitle,omitempty"`      // Short Title
	Url             string `json:"url,omitempty"`             // URL
	AccessDate      string `json:"accessDate,omitempty"`      // Accessed
	Archive         string `json:"archive,omitempty"`         // Archive
	ArchiveLocation string `json:"archiveLocation,omitempty"` // Loc. in Archive
	LibraryCatalog  string `json:"libraryCatalog,omitempty"`  // Library Catalog
	CallNumber      string `json:"callNumber,omitempty"`      // Call Number
	Rights          string `json:"rights,omitempty"`          // Rights
	Extra           string `json:"extra,omitempty"`           // Extra
}

type ItemEmail struct {
	ItemDataBase
	Subject      string `json:"subject,omitempty"`      // Subject
	AbstractNote string `json:"abstractNote,omitempty"` // Abstract
	Date         string `json:"date,omitempty"`         // Date
	ShortTitle   string `json:"shortTitle,omitempty"`   // Short Title
	Url          string `json:"url,omitempty"`          // URL
	AccessDate   string `json:"accessDate,omitempty"`   // Accessed
	Language     string `json:"language,omitempty"`     // Language
	Rights       string `json:"rights,omitempty"`       // Rights
	Extra        string `json:"extra,omitempty"`        // Extra
}

type ItemEncyclopediaArticle struct {
	ItemDataBase
	Title             string `json:"title,omitempty"`             // Title
	AbstractNote      string `json:"abstractNote,omitempty"`      // Abstract
	EncyclopediaTitle string `json:"encyclopediaTitle,omitempty"` // Encyclopedia Title
	Series            string `json:"series,omitempty"`            // Series
	SeriesNumber      string `json:"seriesNumber,omitempty"`      // Series Number
	Volume            string `json:"volume,omitempty"`            // Volume
	NumberOfVolumes   string `json:"numberOfVolumes,omitempty"`   // # of Volumes
	Edition           string `json:"edition,omitempty"`           // Edition
	Place             string `json:"place,omitempty"`             // Place
	Publisher         string `json:"publisher,omitempty"`         // Publisher
	Date              string `json:"date,omitempty"`              // Date
	Pages             string `json:"pages,omitempty"`             // Pages
	ISBN              string `json:"ISBN,omitempty"`              // ISBN
	ShortTitle        string `json:"shortTitle,omitempty"`        // Short Title
	Url               string `json:"url,omitempty"`               // URL
	AccessDate        string `json:"accessDate,omitempty"`        // Accessed
	Language          string `json:"language,omitempty"`          // Language
	Archive           string `json:"archive,omitempty"`           // Archive
	ArchiveLocation   string `json:"archiveLocation,omitempty"`   // Loc. in Archive
	LibraryCatalog    string `json:"libraryCatalog,omitempty"`    // Library Catalog
	CallNumber        string `json:"callNumber,omitempty"`        // Call Number
	Rights            string `json:"rights,omitempty"`            // Rights
	Extra             string `json:"extra,omitempty"`             // Extra
}

type ItemFilm struct {
	ItemDataBase
	Title                string `json:"title,omitempty"`                // Title
	AbstractNote         string `json:"abstractNote,omitempty"`         // Abstract
	Distributor          string `json:"distributor,omitempty"`          // Distributor
	Date                 string `json:"date,omitempty"`                 // Date
	Genre                string `json:"genre,omitempty"`                // Genre
	VideoRecordingFormat string `json:"videoRecordingFormat,omitempty"` // Format
	RunningTime          string `json:"runningTime,omitempty"`          // Running Time
	Language             string `json:"language,omitempty"`             // Language
	ShortTitle           string `json:"shortTitle,omitempty"`           // Short Title
	Url                  string `json:"url,omitempty"`                  // URL
	AccessDate           string `json:"accessDate,omitempty"`           // Accessed
	Archive              string `json:"archive,omitempty"`              // Archive
	ArchiveLocation      string `json:"archiveLocation,omitempty"`      // Loc. in Archive
	LibraryCatalog       string `json:"libraryCatalog,omitempty"`       // Library Catalog
	CallNumber           string `json:"callNumber,omitempty"`           // Call Number
	Rights               string `json:"rights,omitempty"`               // Rights
	Extra                string `json:"extra,omitempty"`                // Extra
}

type ItemForumPost struct {
	ItemDataBase
	Title        string `json:"title,omitempty"`        // Title
	AbstractNote string `json:"abstractNote,omitempty"` // Abstract
	ForumTitle   string `json:"forumTitle,omitempty"`   // Forum/Listserv Title
	PostType     string `json:"postType,omitempty"`     // Post Type
	Date         string `json:"date,omitempty"`         // Date
	Language     string `json:"language,omitempty"`     // Language
	ShortTitle   string `json:"shortTitle,omitempty"`   // Short Title
	Url          string `json:"url,omitempty"`          // URL
	AccessDate   string `json:"accessDate,omitempty"`   // Accessed
	Rights       string `json:"rights,omitempty"`       // Rights
	Extra        string `json:"extra,omitempty"`        // Extra
}

type ItemHearing struct {
	ItemDataBase

	Title           string `json:"title,omitempty"`           // Title
	AbstractNote    string `json:"abstractNote,omitempty"`    // Abstract
	Committee       string `json:"committee,omitempty"`       // Committee
	Place           string `json:"place,omitempty"`           // Place
	Publisher       string `json:"publisher,omitempty"`       // Publisher
	NumberOfVolumes string `json:"numberOfVolumes,omitempty"` // # of Volumes
	DocumentNumber  string `json:"documentNumber,omitempty"`  // Document Number
	Pages           string `json:"pages,omitempty"`           // Pages
	LegislativeBody string `json:"legislativeBody,omitempty"` // Legislative Body
	Session         string `json:"session,omitempty"`         // Session
	History         string `json:"history,omitempty"`         // History
	Date            string `json:"date,omitempty"`            // Date
	Language        string `json:"language,omitempty"`        // Language
	ShortTitle      string `json:"shortTitle,omitempty"`      // Short Title
	Url             string `json:"url,omitempty"`             // URL
	AccessDate      string `json:"accessDate,omitempty"`      // Accessed
	Rights          string `json:"rights,omitempty"`          // Rights
	Extra           string `json:"extra,omitempty"`           // Extra
}

type ItemInstantMessage struct {
	ItemDataBase

	Title        string `json:"title,omitempty"`        // Title
	AbstractNote string `json:"abstractNote,omitempty"` // Abstract
	Date         string `json:"date,omitempty"`         // Date
	Language     string `json:"language,omitempty"`     // Language
	ShortTitle   string `json:"shortTitle,omitempty"`   // Short Title
	Url          string `json:"url,omitempty"`          // URL
	AccessDate   string `json:"accessDate,omitempty"`   // Accessed
	Rights       string `json:"rights,omitempty"`       // Rights
	Extra        string `json:"extra,omitempty"`        // Extra
}

type ItemInterview struct {
	ItemDataBase

	Title           string `json:"title,omitempty"`           // Title
	AbstractNote    string `json:"abstractNote,omitempty"`    // Abstract
	Date            string `json:"date,omitempty"`            // Date
	InterviewMedium string `json:"interviewMedium,omitempty"` // Medium
	Language        string `json:"language,omitempty"`        // Language
	ShortTitle      string `json:"shortTitle,omitempty"`      // Short Title
	Url             string `json:"url,omitempty"`             // URL
	AccessDate      string `json:"accessDate,omitempty"`      // Accessed
	Archive         string `json:"archive,omitempty"`         // Archive
	ArchiveLocation string `json:"archiveLocation,omitempty"` // Loc. in Archive
	LibraryCatalog  string `json:"libraryCatalog,omitempty"`  // Library Catalog
	CallNumber      string `json:"callNumber,omitempty"`      // Call Number
	Rights          string `json:"rights,omitempty"`          // Rights
	Extra           string `json:"extra,omitempty"`           // Extra
}

type ItemJournalArticle struct {
	ItemDataBase
	Title               string `json:"title,omitempty"`               // Title
	AbstractNote        string `json:"abstractNote,omitempty"`        // Abstract
	PublicationTitle    string `json:"publicationTitle,omitempty"`    // Publication
	Volume              string `json:"volume,omitempty"`              // Volume
	Issue               string `json:"issue,omitempty"`               // Issue
	Pages               string `json:"pages,omitempty"`               // Pages
	Date                string `json:"date,omitempty"`                // Date
	Series              string `json:"series,omitempty"`              // Series
	SeriesTitle         string `json:"seriesTitle,omitempty"`         // Series Title
	SeriesText          string `json:"seriesText,omitempty"`          // Series Text
	JournalAbbreviation string `json:"journalAbbreviation,omitempty"` // Journal Abbr
	Language            string `json:"language,omitempty"`            // Language
	DOI                 string `json:"DOI,omitempty"`                 // DOI
	ISSN                string `json:"ISSN,omitempty"`                // ISSN
	ShortTitle          string `json:"shortTitle,omitempty"`          // Short Title
	Url                 string `json:"url,omitempty"`                 // URL
	AccessDate          string `json:"accessDate,omitempty"`          // Accessed
	Archive             string `json:"archive,omitempty"`             // Archive
	ArchiveLocation     string `json:"archiveLocation,omitempty"`     // Loc. in Archive
	LibraryCatalog      string `json:"libraryCatalog,omitempty"`      // Library Catalog
	CallNumber          string `json:"callNumber,omitempty"`          // Call Number
	Rights              string `json:"rights,omitempty"`              // Rights
	Extra               string `json:"extra,omitempty"`               // Extra
}

type ItemLetter struct {
	ItemDataBase

	Title           string `json:"title,omitempty"`           // Title
	AbstractNote    string `json:"abstractNote,omitempty"`    // Abstract
	LetterType      string `json:"letterType,omitempty"`      // Type
	Date            string `json:"date,omitempty"`            // Date
	Language        string `json:"language,omitempty"`        // Language
	ShortTitle      string `json:"shortTitle,omitempty"`      // Short Title
	Url             string `json:"url,omitempty"`             // URL
	AccessDate      string `json:"accessDate,omitempty"`      // Accessed
	Archive         string `json:"archive,omitempty"`         // Archive
	ArchiveLocation string `json:"archiveLocation,omitempty"` // Loc. in Archive
	LibraryCatalog  string `json:"libraryCatalog,omitempty"`  // Library Catalog
	CallNumber      string `json:"callNumber,omitempty"`      // Call Number
	Rights          string `json:"rights,omitempty"`          // Rights
	Extra           string `json:"extra,omitempty"`           // Extra
}

type ItemMagazineArticle struct {
	ItemDataBase
	Title            string `json:"title,omitempty"`            // Title
	AbstractNote     string `json:"abstractNote,omitempty"`     // Abstract
	PublicationTitle string `json:"publicationTitle,omitempty"` // Publication
	Volume           string `json:"volume,omitempty"`           // Volume
	Issue            string `json:"issue,omitempty"`            // Issue
	Date             string `json:"date,omitempty"`             // Date
	Pages            string `json:"pages,omitempty"`            // Pages
	Language         string `json:"language,omitempty"`         // Language
	ISSN             string `json:"ISSN,omitempty"`             // ISSN
	ShortTitle       string `json:"shortTitle,omitempty"`       // Short Title
	Url              string `json:"url,omitempty"`              // URL
	AccessDate       string `json:"accessDate,omitempty"`       // Accessed
	Archive          string `json:"archive,omitempty"`          // Archive
	ArchiveLocation  string `json:"archiveLocation,omitempty"`  // Loc. in Archive
	LibraryCatalog   string `json:"libraryCatalog,omitempty"`   // Library Catalog
	CallNumber       string `json:"callNumber,omitempty"`       // Call Number
	Rights           string `json:"rights,omitempty"`           // Rights
	Extra            string `json:"extra,omitempty"`            // Extra
}

type ItemManuscript struct {
	ItemDataBase

	Title           string `json:"title,omitempty"`           // Title
	AbstractNote    string `json:"abstractNote,omitempty"`    // Abstract
	ManuscriptType  string `json:"manuscriptType,omitempty"`  // Type
	Place           string `json:"place,omitempty"`           // Place
	Date            string `json:"date,omitempty"`            // Date
	NumPages        string `json:"numPages,omitempty"`        // # of Pages
	Language        string `json:"language,omitempty"`        // Language
	ShortTitle      string `json:"shortTitle,omitempty"`      // Short Title
	Url             string `json:"url,omitempty"`             // URL
	AccessDate      string `json:"accessDate,omitempty"`      // Accessed
	Archive         string `json:"archive,omitempty"`         // Archive
	ArchiveLocation string `json:"archiveLocation,omitempty"` // Loc. in Archive
	LibraryCatalog  string `json:"libraryCatalog,omitempty"`  // Library Catalog
	CallNumber      string `json:"callNumber,omitempty"`      // Call Number
	Rights          string `json:"rights,omitempty"`          // Rights
	Extra           string `json:"extra,omitempty"`           // Extra
}

type ItemMap struct {
	ItemDataBase

	Title           string `json:"title,omitempty"`           // Title
	AbstractNote    string `json:"abstractNote,omitempty"`    // Abstract
	MapType         string `json:"mapType,omitempty"`         // Type
	Scale           string `json:"scale,omitempty"`           // Scale
	SeriesTitle     string `json:"seriesTitle,omitempty"`     // Series Title
	Edition         string `json:"edition,omitempty"`         // Edition
	Place           string `json:"place,omitempty"`           // Place
	Publisher       string `json:"publisher,omitempty"`       // Publisher
	Date            string `json:"date,omitempty"`            // Date
	Language        string `json:"language,omitempty"`        // Language
	ISBN            string `json:"ISBN,omitempty"`            // ISBN
	ShortTitle      string `json:"shortTitle,omitempty"`      // Short Title
	Url             string `json:"url,omitempty"`             // URL
	AccessDate      string `json:"accessDate,omitempty"`      // Accessed
	Archive         string `json:"archive,omitempty"`         // Archive
	ArchiveLocation string `json:"archiveLocation,omitempty"` // Loc. in Archive
	LibraryCatalog  string `json:"libraryCatalog,omitempty"`  // Library Catalog
	CallNumber      string `json:"callNumber,omitempty"`      // Call Number
	Rights          string `json:"rights,omitempty"`          // Rights
	Extra           string `json:"extra,omitempty"`           // Extra
}

type ItemNewspaperArticle struct {
	ItemDataBase
	Title            string `json:"title,omitempty"`            // Title
	AbstractNote     string `json:"abstractNote,omitempty"`     // Abstract
	PublicationTitle string `json:"publicationTitle,omitempty"` // Publication
	Place            string `json:"place,omitempty"`            // Place
	Edition          string `json:"edition,omitempty"`          // Edition
	Date             string `json:"date,omitempty"`             // Date
	Section          string `json:"section,omitempty"`          // Section
	Pages            string `json:"pages,omitempty"`            // Pages
	Language         string `json:"language,omitempty"`         // Language
	ShortTitle       string `json:"shortTitle,omitempty"`       // Short Title
	ISSN             string `json:"ISSN,omitempty"`             // ISSN
	Url              string `json:"url,omitempty"`              // URL
	AccessDate       string `json:"accessDate,omitempty"`       // Accessed
	Archive          string `json:"archive,omitempty"`          // Archive
	ArchiveLocation  string `json:"archiveLocation,omitempty"`  // Loc. in Archive
	LibraryCatalog   string `json:"libraryCatalog,omitempty"`   // Library Catalog
	CallNumber       string `json:"callNumber,omitempty"`       // Call Number
	Rights           string `json:"rights,omitempty"`           // Rights
	Extra            string `json:"extra,omitempty"`            // Extra
}

type ItemPatent struct {
	ItemDataBase
	Title             string `json:"title,omitempty"`             // Title
	AbstractNote      string `json:"abstractNote,omitempty"`      // Abstract
	Place             string `json:"place,omitempty"`             // Place
	Country           string `json:"country,omitempty"`           // Country
	Assignee          string `json:"assignee,omitempty"`          // Assignee
	IssuingAuthority  string `json:"issuingAuthority,omitempty"`  // Issuing Authority
	PatentNumber      string `json:"patentNumber,omitempty"`      // Patent Number
	FilingDate        string `json:"filingDate,omitempty"`        // Filing Date
	Pages             string `json:"pages,omitempty"`             // Pages
	ApplicationNumber string `json:"applicationNumber,omitempty"` // Application Number
	PriorityNumbers   string `json:"priorityNumbers,omitempty"`   // Priority Numbers
	IssueDate         string `json:"issueDate,omitempty"`         // Issue Date
	References        string `json:"references,omitempty"`        // References
	LegalStatus       string `json:"legalStatus,omitempty"`       // Legal Status
	Language          string `json:"language,omitempty"`          // Language
	ShortTitle        string `json:"shortTitle,omitempty"`        // Short Title
	Url               string `json:"url,omitempty"`               // URL
	AccessDate        string `json:"accessDate,omitempty"`        // Accessed
	Rights            string `json:"rights,omitempty"`            // Rights
	Extra             string `json:"extra,omitempty"`             // Extra
}

type ItemPodcast struct {
	ItemDataBase
	Title         string `json:"title,omitempty"`         // Title
	AbstractNote  string `json:"abstractNote,omitempty"`  // Abstract
	SeriesTitle   string `json:"seriesTitle,omitempty"`   // Series Title
	EpisodeNumber string `json:"episodeNumber,omitempty"` // Episode Number
	AudioFileType string `json:"audioFileType,omitempty"` // File Type
	RunningTime   string `json:"runningTime,omitempty"`   // Running Time
	Url           string `json:"url,omitempty"`           // URL
	AccessDate    string `json:"accessDate,omitempty"`    // Accessed
	Language      string `json:"language,omitempty"`      // Language
	ShortTitle    string `json:"shortTitle,omitempty"`    // Short Title
	Rights        string `json:"rights,omitempty"`        // Rights
	Extra         string `json:"extra,omitempty"`         // Extra
}

type ItemPresentation struct {
	ItemDataBase
	Title            string `json:"title,omitempty"`            // Title
	AbstractNote     string `json:"abstractNote,omitempty"`     // Abstract
	PresentationType string `json:"presentationType,omitempty"` // Type
	Date             string `json:"date,omitempty"`             // Date
	Place            string `json:"place,omitempty"`            // Place
	MeetingName      string `json:"meetingName,omitempty"`      // Meeting Name
	Url              string `json:"url,omitempty"`              // URL
	AccessDate       string `json:"accessDate,omitempty"`       // Accessed
	Language         string `json:"language,omitempty"`         // Language
	ShortTitle       string `json:"shortTitle,omitempty"`       // Short Title
	Rights           string `json:"rights,omitempty"`           // Rights
	Extra            string `json:"extra,omitempty"`            // Extra
}

type ItemRadioBroadcast struct {
	ItemDataBase
	Title                string `json:"title,omitempty"`                // Title
	AbstractNote         string `json:"abstractNote,omitempty"`         // Abstract
	ProgramTitle         string `json:"programTitle,omitempty"`         // Program Title
	EpisodeNumber        string `json:"episodeNumber,omitempty"`        // Episode Number
	AudioRecordingFormat string `json:"audioRecordingFormat,omitempty"` // Format
	Place                string `json:"place,omitempty"`                // Place
	Network              string `json:"network,omitempty"`              // Network
	Date                 string `json:"date,omitempty"`                 // Date
	RunningTime          string `json:"runningTime,omitempty"`          // Running Time
	Language             string `json:"language,omitempty"`             // Language
	ShortTitle           string `json:"shortTitle,omitempty"`           // Short Title
	Url                  string `json:"url,omitempty"`                  // URL
	AccessDate           string `json:"accessDate,omitempty"`           // Accessed
	Archive              string `json:"archive,omitempty"`              // Archive
	ArchiveLocation      string `json:"archiveLocation,omitempty"`      // Loc. in Archive
	LibraryCatalog       string `json:"libraryCatalog,omitempty"`       // Library Catalog
	CallNumber           string `json:"callNumber,omitempty"`           // Call Number
	Rights               string `json:"rights,omitempty"`               // Rights
	Extra                string `json:"extra,omitempty"`                // Extra
}

type ItemReport struct {
	ItemDataBase

	Title           string `json:"title,omitempty"`           // Title
	AbstractNote    string `json:"abstractNote,omitempty"`    // Abstract
	ReportNumber    string `json:"reportNumber,omitempty"`    // Report Number
	ReportType      string `json:"reportType,omitempty"`      // Report Type
	SeriesTitle     string `json:"seriesTitle,omitempty"`     // Series Title
	Place           string `json:"place,omitempty"`           // Place
	Institution     string `json:"institution,omitempty"`     // Institution
	Date            string `json:"date,omitempty"`            // Date
	Pages           string `json:"pages,omitempty"`           // Pages
	Language        string `json:"language,omitempty"`        // Language
	ShortTitle      string `json:"shortTitle,omitempty"`      // Short Title
	Url             string `json:"url,omitempty"`             // URL
	AccessDate      string `json:"accessDate,omitempty"`      // Accessed
	Archive         string `json:"archive,omitempty"`         // Archive
	ArchiveLocation string `json:"archiveLocation,omitempty"` // Loc. in Archive
	LibraryCatalog  string `json:"libraryCatalog,omitempty"`  // Library Catalog
	CallNumber      string `json:"callNumber,omitempty"`      // Call Number
	Rights          string `json:"rights,omitempty"`          // Rights
	Extra           string `json:"extra,omitempty"`           // Extra
}

type ItemStatute struct {
	ItemDataBase

	NameOfAct       string `json:"nameOfAct,omitempty"`       // Name of Act
	AbstractNote    string `json:"abstractNote,omitempty"`    // Abstract
	Code            string `json:"code,omitempty"`            // Code
	CodeNumber      string `json:"codeNumber,omitempty"`      // Code Number
	PublicLawNumber string `json:"publicLawNumber,omitempty"` // Public Law Number
	DateEnacted     string `json:"dateEnacted,omitempty"`     // Date Enacted
	Pages           string `json:"pages,omitempty"`           // Pages
	Section         string `json:"section,omitempty"`         // Section
	Session         string `json:"session,omitempty"`         // Session
	History         string `json:"history,omitempty"`         // History
	Language        string `json:"language,omitempty"`        // Language
	ShortTitle      string `json:"shortTitle,omitempty"`      // Short Title
	Url             string `json:"url,omitempty"`             // URL
	AccessDate      string `json:"accessDate,omitempty"`      // Accessed
	Rights          string `json:"rights,omitempty"`          // Rights
	Extra           string `json:"extra,omitempty"`           // Extra
}

type ItemTvBroadcast struct {
	ItemDataBase
	Title                string `json:"title,omitempty"`                // Title
	AbstractNote         string `json:"abstractNote,omitempty"`         // Abstract
	ProgramTitle         string `json:"programTitle,omitempty"`         // Program Title
	EpisodeNumber        string `json:"episodeNumber,omitempty"`        // Episode Number
	VideoRecordingFormat string `json:"videoRecordingFormat,omitempty"` // Format
	Place                string `json:"place,omitempty"`                // Place
	Network              string `json:"network,omitempty"`              // Network
	Date                 string `json:"date,omitempty"`                 // Date
	RunningTime          string `json:"runningTime,omitempty"`          // Running Time
	Language             string `json:"language,omitempty"`             // Language
	ShortTitle           string `json:"shortTitle,omitempty"`           // Short Title
	Url                  string `json:"url,omitempty"`                  // URL
	AccessDate           string `json:"accessDate,omitempty"`           // Accessed
	Archive              string `json:"archive,omitempty"`              // Archive
	ArchiveLocation      string `json:"archiveLocation,omitempty"`      // Loc. in Archive
	LibraryCatalog       string `json:"libraryCatalog,omitempty"`       // Library Catalog
	CallNumber           string `json:"callNumber,omitempty"`           // Call Number
	Rights               string `json:"rights,omitempty"`               // Rights
	Extra                string `json:"extra,omitempty"`                // Extra
}

type ItemThesis struct {
	ItemDataBase

	Title           string `json:"title,omitempty"`           // Title
	AbstractNote    string `json:"abstractNote,omitempty"`    // Abstract
	ThesisType      string `json:"thesisType,omitempty"`      // Type
	University      string `json:"university,omitempty"`      // University
	Place           string `json:"place,omitempty"`           // Place
	Date            string `json:"date,omitempty"`            // Date
	NumPages        string `json:"numPages,omitempty"`        // # of Pages
	Language        string `json:"language,omitempty"`        // Language
	ShortTitle      string `json:"shortTitle,omitempty"`      // Short Title
	Url             string `json:"url,omitempty"`             // URL
	AccessDate      string `json:"accessDate,omitempty"`      // Accessed
	Archive         string `json:"archive,omitempty"`         // Archive
	ArchiveLocation string `json:"archiveLocation,omitempty"` // Loc. in Archive
	LibraryCatalog  string `json:"libraryCatalog,omitempty"`  // Library Catalog
	CallNumber      string `json:"callNumber,omitempty"`      // Call Number
	Rights          string `json:"rights,omitempty"`          // Rights
	Extra           string `json:"extra,omitempty"`           // Extra
}

type ItemVideoRecording struct {
	ItemDataBase
	Title                string `json:"title,omitempty"`                // Title
	AbstractNote         string `json:"abstractNote,omitempty"`         // Abstract
	VideoRecordingFormat string `json:"videoRecordingFormat,omitempty"` // Format
	SeriesTitle          string `json:"seriesTitle,omitempty"`          // Series Title
	Volume               string `json:"volume,omitempty"`               // Volume
	NumberOfVolumes      string `json:"numberOfVolumes,omitempty"`      // # of Volumes
	Place                string `json:"place,omitempty"`                // Place
	Studio               string `json:"studio,omitempty"`               // Studio
	Date                 string `json:"date,omitempty"`                 // Date
	RunningTime          string `json:"runningTime,omitempty"`          // Running Time
	Language             string `json:"language,omitempty"`             // Language
	ISBN                 string `json:"ISBN,omitempty"`                 // ISBN
	ShortTitle           string `json:"shortTitle,omitempty"`           // Short Title
	Url                  string `json:"url,omitempty"`                  // URL
	AccessDate           string `json:"accessDate,omitempty"`           // Accessed
	Archive              string `json:"archive,omitempty"`              // Archive
	ArchiveLocation      string `json:"archiveLocation,omitempty"`      // Loc. in Archive
	LibraryCatalog       string `json:"libraryCatalog,omitempty"`       // Library Catalog
	CallNumber           string `json:"callNumber,omitempty"`           // Call Number
	Rights               string `json:"rights,omitempty"`               // Rights
	Extra                string `json:"extra,omitempty"`                // Extra
}

type ItemWebpage struct {
	ItemDataBase

	Title        string `json:"title,omitempty"`        // Title
	AbstractNote string `json:"abstractNote,omitempty"` // Abstract
	WebsiteTitle string `json:"websiteTitle,omitempty"` // Website Title
	WebsiteType  string `json:"websiteType,omitempty"`  // Website Type
	Date         string `json:"date,omitempty"`         // Date
	ShortTitle   string `json:"shortTitle,omitempty"`   // Short Title
	Url          string `json:"url,omitempty"`          // URL
	AccessDate   string `json:"accessDate,omitempty"`   // Accessed
	Language     string `json:"language,omitempty"`     // Language
	Rights       string `json:"rights,omitempty"`       // Rights
	Extra        string `json:"extra,omitempty"`        // Extra
}
