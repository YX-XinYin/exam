package main

import (
	"context"
	"flag"
	"fmt"
	"os"
)

func main() {
	questionPath := flag.String("questions", "题目.docx", "题目 docx")
	templatePath := flag.String("template", "答案模板.docx", "答案模板 docx")
	packPath := flag.String("pack", "参赛包.docx", "参赛包 docx")
	outPath := flag.String("out", "答案.json", "输出答案 JSON")
	name := flag.String("name", "", "选手名称")
	flag.Parse()

	if err := run(*questionPath, *templatePath, *packPath, *outPath, *name); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(questionPath, templatePath, packPath, outPath, name string) error {
	bundle, err := LoadBundle(questionPath, templatePath, packPath)
	if err != nil {
		return err
	}
	assets := newLiveAssets()
	answers := make([]Answer, 0, len(bundle.Questions))
	refused := 0
	for _, q := range bundle.Questions {
		query, err := Convert(context.Background(), q, assets)
		if err != nil {
			return err
		}
		if query == Unconvertible {
			refused++
		}
		answers = append(answers, Answer{ID: q.ID, Query: query})
	}
	if len(answers) != len(bundle.Questions) {
		return fmt.Errorf("答案数量 %d 与题目 %d 不一致", len(answers), len(bundle.Questions))
	}
	if err := writeAnswer(outPath, name, bundle.PackageID, answers); err != nil {
		return err
	}
	fmt.Printf("参赛包 %s，共 %d 题，其中 %d 题不能转换，已写入 %s\n", bundle.PackageID, len(answers), refused, outPath)
	if name == "" {
		fmt.Println("选手名称仍为空，提交前用 -name 填上")
	}
	return nil
}
