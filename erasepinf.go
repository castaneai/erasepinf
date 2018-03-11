package main

import (
	"log"
	"os"
	"encoding/binary"
	"encoding/hex"
	"io"
	"errors"
	"fmt"
)

// atomName => padding size
var boxAtomPaddings = map[string]int64{
	"moov": 0,
	"trak": 0,
	"mdia": 0,
	"minf": 0,
	"stsd": 8,
	"stbl": 0,
	"mp4a": 28,

	// --- in pinf
	"pinf": 0,
	"schi": 0,
}

func main() {
	if len(os.Args) < 2 {
		log.Fatalf("Usage: ./erasepinf <mp4 filename>")
	}

	filename := os.Args[1]
	f, err := os.OpenFile(filename, os.O_RDWR, 0644)
	defer f.Close()
	if err != nil {
		log.Fatalln(err)
	}

	atoms, err := readAllAtoms(f)
	if err != nil {
		log.Fatalln(err)
	}

	pinf, err := searchAtom(atoms, "pinf")
	if err != nil {
		log.Fatalln(err)
	}

	dumper := hex.Dumper(os.Stdout)
	defer dumper.Close()

	data, err := pinf.ReadData(f)
	if err != nil {
		log.Fatalln(err)
	}

	fmt.Println("-------------before-----------------")
	dumper.Write(data)

	if err := pinf.destroy(f); err != nil {
		log.Fatalln(err)
	}

	data, err = pinf.ReadData(f)
	if err != nil {
		log.Fatalln(err)
	}

	fmt.Println("-------------after-----------------")
	dumper.Write(data)
}

func searchAtom(atoms map[string]*Atom, target string) (*Atom, error) {
	for name, atom := range atoms {
		if name == target {
			return atom, nil
		}
		if len(atom.Children) > 0 {
			return searchAtom(atom.Children, target)
		}
	}
	return nil, errors.New("not found")
}

type Atom struct {
	Name string
	DataStartPos int64
	DataLen int64
	Children map[string]*Atom
}

func (a *Atom) ReadData(r io.ReadSeeker) ([]byte, error) {
	if _, err := r.Seek(a.DataStartPos, io.SeekStart); err != nil {
		return nil, err
	}
	buf := make([]byte, a.DataLen)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, err
	}
	return buf, nil
}

func (a *Atom) destroy(w io.WriteSeeker) (error) {
	if _, err := w.Seek(a.DataStartPos, io.SeekStart); err != nil {
		return err
	}
	pad := make([]byte, a.DataLen)
	if _, err := w.Write(pad); err != nil {
		return err
	}
	return nil
}

func readAllAtoms(r io.ReadSeeker) (map[string]*Atom, error) {
	atoms := make(map[string]*Atom)
	for {
		atom, err := readAtom(r)
		if err != nil {
			if err != io.EOF {
				return nil, err
			}
			break
		}
		atoms[atom.Name] = atom
	}
	return atoms, nil
}

func readAtom(r io.ReadSeeker) (*Atom, error) {
	var length uint32
	if err := binary.Read(r, binary.BigEndian, &length); err != nil {
		return nil, err
	}

	nameb := make([]byte, 4)
	if err := binary.Read(r, binary.BigEndian, &nameb); err != nil {
		return nil, err
	}
	name := string(nameb)

	startPos, err := r.Seek(0, io.SeekCurrent)
	if err !=  nil {
		return nil, err
	}
	dataLen := int64(length - 8)

	catoms := make(map[string]*Atom)
	pad, isBox := boxAtomPaddings[name]
	if isBox {
		r.Seek(pad, io.SeekCurrent)
		for {
			catom, err := readAtom(r)
			if err != nil {
				return nil, err
			}
			catoms[catom.Name] = catom

			cur, err := r.Seek(0, io.SeekCurrent)
			if err != nil {
				return nil, err
			}
			if cur >= startPos + dataLen {
				break
			}
		}
	} else {
		if _, err := r.Seek(dataLen, io.SeekCurrent); err != nil {
			return nil, err
		}
	}

	return &Atom {
		Name: name,
		DataStartPos: startPos,
		DataLen: dataLen,
		Children: catoms,
	}, nil
}
