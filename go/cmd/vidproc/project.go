package main

import (
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"vidproc/internal/project"
	"vidproc/internal/store"
)

// projectDBPath 返回 CLI 数据目录中的 SQLite 路径。
func projectDBPath() string {
	base, err := os.UserConfigDir()
	if err != nil {
		base = os.TempDir()
	}
	return filepath.Join(base, "vidproc", "vidproc.db")
}

func openProjectDB() (*sql.DB, error) {
	db, err := store.Open(projectDBPath())
	if err != nil {
		return nil, err
	}
	if err := store.Migrate(db, project.ProjectSchema); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

// cmdProject 管理持久化项目(对齐 HomeCast store + 垂直切片)。
func cmdProject(args []string) error {
	if len(args) < 1 {
		return errors.New("project: need subcommand (new|list|get|update|delete|render)")
	}
	cmd := args[0]
	rest := args[1:]

	fs := flag.NewFlagSet("project "+cmd, flag.ContinueOnError)
	name := fs.String("name", "", "project name")
	spec := fs.String("spec", "", "path to project.yaml spec file")
	out := fs.String("o", "", "output path")
	if err := fs.Parse(rest); err != nil {
		return err
	}

	db, err := openProjectDB()
	if err != nil {
		return err
	}
	defer db.Close()

	switch cmd {
	case "new":
		if *spec == "" {
			return errors.New("project new: -spec <project.yaml> required")
		}
		data, err := os.ReadFile(*spec)
		if err != nil {
			return fmt.Errorf("project new: read spec: %w", err)
		}
		projectName := *name
		if projectName == "" {
			projectName = filepath.Base(*spec)
		}
		id, err := project.Create(db, projectName, string(data), *out)
		if err != nil {
			return err
		}
		fmt.Printf("project %d created: %s\n", id, projectName)

	case "list":
		projects, err := project.List(db)
		if err != nil {
			return err
		}
		if len(projects) == 0 {
			fmt.Println("no projects")
			return nil
		}
		for _, p := range projects {
			fmt.Printf("%d\t%s\t%s\n", p.ID, p.Name, p.OutputPath)
		}

	case "get":
		if len(fs.Args()) < 1 {
			return errors.New("project get: need <id>")
		}
		var id int64
		if _, err := fmt.Sscanf(fs.Arg(0), "%d", &id); err != nil {
			return fmt.Errorf("project get: bad id %q", fs.Arg(0))
		}
		p, err := project.Get(db, id)
		if err != nil {
			return err
		}
		fmt.Printf("ID: %d\nName: %s\nOutput: %s\nCreated: %s\nUpdated: %s\n--- spec ---\n%s\n",
			p.ID, p.Name, p.OutputPath, p.CreatedAt.Format("2006-01-02 15:04:05"), p.UpdatedAt.Format("2006-01-02 15:04:05"), p.SpecYAML)

	case "update":
		if len(fs.Args()) < 1 {
			return errors.New("project update: need <id> [-spec <file>] [-o <out>]")
		}
		var id int64
		if _, err := fmt.Sscanf(fs.Arg(0), "%d", &id); err != nil {
			return fmt.Errorf("project update: bad id %q", fs.Arg(0))
		}
		p, err := project.Get(db, id)
		if err != nil {
			return err
		}
		newSpec, newOut := p.SpecYAML, p.OutputPath
		if *spec != "" {
			data, err := os.ReadFile(*spec)
			if err != nil {
				return err
			}
			newSpec = string(data)
		}
		if *out != "" {
			newOut = *out
		}
		if err := project.Update(db, id, newSpec, newOut); err != nil {
			return err
		}
		fmt.Printf("project %d updated\n", id)

	case "delete":
		if len(fs.Args()) < 1 {
			return errors.New("project delete: need <id>")
		}
		var id int64
		if _, err := fmt.Sscanf(fs.Arg(0), "%d", &id); err != nil {
			return fmt.Errorf("project delete: bad id %q", fs.Arg(0))
		}
		if err := project.Delete(db, id); err != nil {
			return err
		}
		fmt.Printf("project %d deleted\n", id)

	case "render":
		if len(fs.Args()) < 1 {
			return errors.New("project render: need <id>")
		}
		var id int64
		if _, err := fmt.Sscanf(fs.Arg(0), "%d", &id); err != nil {
			return fmt.Errorf("project render: bad id %q", fs.Arg(0))
		}
		p, err := project.Get(db, id)
		if err != nil {
			return err
		}
		if p.SpecYAML == "" {
			return project.ErrNoSpec
		}
		// 落盘临时 spec 并交给 render 命令
		tmpSpec := filepath.Join(os.TempDir(), fmt.Sprintf("vidproc_project_%d.yaml", id))
		if err := project.SaveSpecToFile(tmpSpec, p.SpecYAML); err != nil {
			return err
		}
		defer os.Remove(tmpSpec)
		renderArgs := []string{tmpSpec}
		if p.OutputPath != "" {
			renderArgs = append(renderArgs, "-o", p.OutputPath)
		}
		return cmdRender(renderArgs)

	default:
		return fmt.Errorf("project: unknown subcommand %q", cmd)
	}
	return nil
}
