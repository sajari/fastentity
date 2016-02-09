package fastentity

import "testing"

var resume_store *Store

func TestHash(t *testing.T) {
	strs := map[string]string{
		"golang developer":   "gol016",
		"San Francisco, USA": "san018",
		"PHP":                "php003",
		"本語":                 "本語002",
		"C":                  "c001", // Single char entity
	}
	for original, hashexpected := range strs {
		hashed := hash([]rune(original))
		if hashexpected != hashed {
			t.Errorf("Failed to hash  %s (%s)", original, hashed)
		}
	}
}

func TestFind(t *testing.T) {
	str := []rune("日 本語. jack was a golang developer from sydney, for someone. San Francisco, USA... Or so they say. Maybe PHP, or PDX. Jody Shipway\\u0007\\n\\u0007")

	store := New("locations", "jobTitles") // Intentionally not initialising "skills"
	store.Add("locations", []rune("San Francisco, USA"))
	store.Add("jobTitles", []rune("golang developer"))
	store.Add("skills", []rune("PHP"), []rune("本語"), []rune("PRC"))
	store.Add("last", []rune("shipway")) // Intentionally adding garbled unicode

	results := store.FindAll(str)
	for group, found := range results {
		switch group {
		case "locations":
			ok := false
			for _, f := range found {
				if string(f.Text) == "San Francisco, USA" && f.Offset == 60 {
					ok = true
				}
			}
			if !ok {
				t.Errorf("Failed to find location entity 'San Francisco, USA'")
			}
		case "skills":
			ok, ok2 := false, false
			for _, f := range found {
				if string(f.Text) == "PHP" && f.Offset == 104 {
					ok = true
				}
				if string(f.Text) == "本語" && f.Offset == 2 {
					ok2 = true
				}
			}
			if !ok {
				t.Errorf("Failed to find skill entity 'PHP'")
			}
			if !ok2 {
				t.Errorf("Failed to find skill entity '本語'")
			}
		case "jobTitles":
			ok := false
			for _, f := range found {
				if string(f.Text) == "golang developer" && f.Offset == 17 {
					ok = true
				}
			}
			if !ok {
				t.Errorf("Failed to find jobTitle entity 'golang developer'")
			}
		case "last":
			ok := false
			for _, f := range found {
				if string(f.Text) == "Shipway" && f.Offset == 122 {
					ok = true
				}
			}
			if !ok {
				t.Errorf("Failed to find last name entity 'Shipway'")
			}
		}
	}
}

// Approximates finding entities in a resume size document
func BenchmarkFind(b *testing.B) {
	b.StopTimer()
	str := []rune("Jim Smith,  Bleeker Street Houston, Texas 77034  (315) 555-5145  jimsmith@example.com  Objective: Seeking a position in an accounting field where I can utilize my skills and abilities in the field of tax oriented job that offers professional tax accountant.  Educational Details:  Bachelor of Science in Accounting University of Houston, 1989 Master of Science of Taxation University of New York, 1990  Master of Business Administration in Finance University of New York, 1992  Summary of Qualifications:  •  6+ years of tax and accounting experience.  •  Experience in establishing corporate tax department.  •  Experience working in global business environment.  •  Experience in using technology tools to leverage data, increase process and tax return efficiency, and complete work.  •  Able to research tax issues, apply practical tax experience.  Skills:  •  Excellent technical writing and editing skills.  •  Strong verbal communication skills.  •  Strong influencing skills across business functions.  •  Advanced computer skills.  •  Excellent accounting skills.  Computer Skills:  Lotus, Excel, Ami Pro, WordPerfect, ProComm Plus, Spreadsheet Auditor, Flowcharting III. Professional Experience:  Leading Commercial Printer, Houston, TX, 1996-2000 Tax Accountant  Responsibilities:  •  Prepared individual, partnership, corporate and other types of tax returns.  •  Did research on various tax matters.  •  Ensured that all sales and use tax returns are filed timely and accurately.  •  Prepared written communication for sales and use tax issues.  •  Collected information for all sales tax, use tax, and personal property tax audits.  •  Performed other duties as assigned.  Hipping Agency, Friendswood, TX, 1992-1995  Tax Staff Accountant  Responsibilities:  •  Established a 401K plan for company employees, enhancing the company's benefits package.  •  Prepared payroll, sales, use & property & commercial rent returns.  •  Responded to both client and government inquiries.  •  Devised the spreadsheet packages, financial statements and tax filings.  •  Ensured that all legal fees and push down entries for separate companies are recorded  ")
	b.StartTimer()
	for n := 0; n < b.N; n++ {
		resume_store.FindAll(str)
	}
}

func TestSaveLoad(t *testing.T) {
	store := New("locations", "jobTitles", "skills")
	store.Add("locations", []rune("San Francisco, USA"))
	store.Add("jobTitles", []rune("golang developer"))
	store.Add("skills", []rune("PHP"), []rune("本語"), []rune("PRC"))
	err := store.Save("/tmp")
	if err != nil {
		t.Errorf("Failed to save store")
	}

	store, err = FromDir("/tmp")
	if err != nil {
		t.Errorf("Failed to load store from disk")
	}
	for name, _ := range store.groups {
		if name != "locations" && name != "jobTitles" && name != "skills" {
			t.Errorf("Groups were not named what they should be. Got '%s'", name)
		}
	}
}
